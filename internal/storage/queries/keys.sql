-- name: GetKeyOwnerEnabled :one
SELECT enabled FROM users WHERE id = sqlc.arg(user_id);

-- name: CountActiveKeys :one
SELECT count(*) FROM api_keys WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: CreateKey :execlastid
INSERT INTO api_keys(user_id, name, prefix, token_hash, created_at)
VALUES (sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(prefix), sqlc.arg(token_hash), sqlc.arg(created_at));

-- name: ListKeys :many
SELECT id, name, prefix, created_at, last_used_at, revoked_at FROM api_keys
WHERE user_id = sqlc.arg(user_id) AND (id < sqlc.arg(before_id) OR sqlc.arg(before_id) = 0)
ORDER BY id DESC LIMIT 51;

-- name: RevokeKey :one
UPDATE api_keys SET revoked_at = COALESCE(revoked_at, sqlc.arg(now))
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id)
RETURNING id, name, prefix, created_at, last_used_at, revoked_at;

-- name: AuthenticateKey :one
SELECT k.id, k.user_id FROM api_keys k JOIN users u ON u.id = k.user_id
WHERE k.token_hash = sqlc.arg(token_hash) AND k.revoked_at IS NULL AND u.enabled = 1;

-- name: TouchKey :exec
UPDATE api_keys SET last_used_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL
AND (last_used_at IS NULL OR last_used_at <= sqlc.arg(threshold));
