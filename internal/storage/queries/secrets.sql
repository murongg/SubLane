-- name: SaveKeySecret :exec
UPDATE api_keys SET encrypted_secret=sqlc.arg(secret) WHERE id=sqlc.arg(key_id);

-- name: GetKeySecret :one
SELECT k.id,k.user_id,k.token_hash,k.revoked_at,k.encrypted_secret AS secret
FROM api_keys k JOIN users u ON u.id=k.user_id JOIN account_groups g ON g.id=k.group_id
WHERE k.id=sqlc.arg(id) AND k.user_id=sqlc.arg(user_id) AND g.tenant_id=sqlc.arg(tenant_id) AND u.enabled=1;

-- name: ListKeySecrets :many
SELECT id,user_id,token_hash,encrypted_secret AS secret FROM api_keys
WHERE encrypted_secret IS NOT NULL AND id>sqlc.arg(after_id) ORDER BY id LIMIT 100;

-- name: CountEncryptedKeys :one
SELECT count(*) FROM api_keys WHERE encrypted_secret IS NOT NULL;
