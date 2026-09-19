-- name: GetKeyOwnerEnabled :one
SELECT enabled FROM users WHERE id = sqlc.arg(user_id);

-- name: CountActiveKeys :one
SELECT count(*) FROM api_keys WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: CreateKey :execlastid
INSERT INTO api_keys(user_id,group_id,name,prefix,token_hash,created_at)
VALUES(sqlc.arg(user_id),sqlc.arg(group_id),sqlc.arg(name),sqlc.arg(prefix),sqlc.arg(token_hash),sqlc.arg(created_at));

-- name: ListKeys :many
SELECT k.id,k.group_id,g.name AS group_name,
CASE WHEN g.enabled=1 AND u.enabled=1 AND (u.role='admin' OR EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=g.id AND m.user_id=u.id)) THEN 'allowed' ELSE 'blocked' END AS group_access,
k.name,k.prefix,k.created_at,k.last_used_at,k.revoked_at
FROM api_keys k JOIN account_groups g ON g.id=k.group_id JOIN users u ON u.id=k.user_id
WHERE k.user_id=sqlc.arg(user_id) AND (k.id<sqlc.arg(before_id) OR sqlc.arg(before_id)=0)
ORDER BY k.id DESC LIMIT 51;

-- name: GetKey :one
SELECT k.id,k.group_id,g.name AS group_name,
CASE WHEN g.enabled=1 AND u.enabled=1 AND (u.role='admin' OR EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=g.id AND m.user_id=u.id)) THEN 'allowed' ELSE 'blocked' END AS group_access,
k.name,k.prefix,k.created_at,k.last_used_at,k.revoked_at
FROM api_keys k JOIN account_groups g ON g.id=k.group_id JOIN users u ON u.id=k.user_id
WHERE k.id=sqlc.arg(id) AND k.user_id=sqlc.arg(user_id);

-- name: RevokeKey :execrows
UPDATE api_keys SET revoked_at=COALESCE(revoked_at,sqlc.arg(now)) WHERE id=sqlc.arg(id) AND user_id=sqlc.arg(user_id);

-- name: AuthenticateKey :one
SELECT k.id,k.user_id,k.group_id FROM api_keys k JOIN users u ON u.id=k.user_id JOIN account_groups g ON g.id=k.group_id
WHERE k.token_hash=sqlc.arg(token_hash) AND k.revoked_at IS NULL AND u.enabled=1 AND g.enabled=1
AND (u.role='admin' OR EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=g.id AND m.user_id=u.id));

-- name: TouchKey :exec
UPDATE api_keys SET last_used_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL
AND (last_used_at IS NULL OR last_used_at <= sqlc.arg(threshold));
