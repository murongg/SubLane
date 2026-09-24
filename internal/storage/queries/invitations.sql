-- name: CreateInvitation :one
INSERT INTO invitations(token_hash, tenant_id, created_by, created_at, expires_at)
VALUES (sqlc.arg(token_hash), sqlc.arg(tenant_id), sqlc.arg(created_by), sqlc.arg(created_at), sqlc.arg(expires_at))
RETURNING id;

-- name: GetUsableInvitation :one
SELECT i.id FROM invitations i JOIN tenants t ON t.id=i.tenant_id
WHERE i.token_hash=sqlc.arg(token_hash) AND i.tenant_id=sqlc.arg(tenant_id)
AND i.expires_at>sqlc.arg(now) AND i.used_at IS NULL AND t.status='active';

-- name: ConsumeInvitation :execrows
UPDATE invitations SET used_at=sqlc.arg(used_at), used_by=sqlc.arg(used_by)
WHERE id=sqlc.arg(id) AND used_at IS NULL AND expires_at>sqlc.arg(used_at);
