-- name: ListTenantMembers :many
SELECT u.id,u.username,m.role,m.enabled,u.enabled AS user_enabled,m.created_at
FROM memberships m JOIN users u ON u.id=m.user_id
WHERE m.tenant_id=sqlc.arg(tenant_id) AND m.removed_at IS NULL AND m.role IN ('owner','member','admin')
AND (u.id<sqlc.arg(before_id) OR sqlc.arg(before_id)=0)
ORDER BY u.id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountTenantMembers :one
SELECT COUNT(*) FROM memberships m JOIN users u ON u.id=m.user_id
WHERE m.tenant_id=sqlc.arg(tenant_id) AND m.removed_at IS NULL AND m.role IN ('owner','member','admin');

-- name: CanManageTenant :one
SELECT EXISTS(SELECT 1 FROM memberships m JOIN tenants t ON t.id=m.tenant_id JOIN users u ON u.id=m.user_id
WHERE m.tenant_id=sqlc.arg(tenant_id) AND m.user_id=sqlc.arg(actor_id)
AND m.role IN ('owner','admin') AND m.enabled=1 AND u.enabled=1 AND t.status='active');

-- name: SetTenantMemberEnabled :execrows
UPDATE memberships SET enabled=sqlc.arg(enabled)
WHERE tenant_id=sqlc.arg(tenant_id) AND user_id=sqlc.arg(user_id) AND role='member' AND removed_at IS NULL;

-- name: UpdateTenantMemberRole :exec
UPDATE memberships SET role=sqlc.arg(role)
WHERE tenant_id=sqlc.arg(tenant_id) AND user_id=sqlc.arg(user_id) AND role IN ('member','admin') AND removed_at IS NULL;

-- name: GetTenantMember :one
SELECT u.id,u.username,m.role,m.enabled,u.enabled AS user_enabled,m.created_at,m.removed_at
FROM memberships m JOIN users u ON u.id=m.user_id
WHERE m.tenant_id=sqlc.arg(tenant_id) AND m.user_id=sqlc.arg(user_id);

-- name: AddTenantMember :execrows
INSERT INTO memberships(tenant_id,user_id,role,created_at)
SELECT sqlc.arg(tenant_id),id,sqlc.arg(role),sqlc.arg(created_at) FROM users WHERE id=sqlc.arg(user_id) AND enabled=1
ON CONFLICT(tenant_id,user_id) DO UPDATE SET role=excluded.role,enabled=1,
requests_per_minute=CASE WHEN memberships.removed_at IS NOT NULL THEN 0 ELSE memberships.requests_per_minute END,
max_concurrency=CASE WHEN memberships.removed_at IS NOT NULL THEN 10 ELSE memberships.max_concurrency END,
first_request_at=CASE WHEN memberships.removed_at IS NOT NULL THEN 0 ELSE memberships.first_request_at END,
created_at=CASE WHEN memberships.removed_at IS NOT NULL THEN excluded.created_at ELSE memberships.created_at END,
removed_at=NULL;

-- name: RemoveTenantMember :exec
UPDATE memberships SET enabled=0,removed_at=sqlc.arg(now)
WHERE tenant_id=sqlc.arg(tenant_id) AND user_id=sqlc.arg(user_id) AND role!='owner' AND removed_at IS NULL;

-- name: RevokeTenantMemberKeys :exec
UPDATE api_keys SET revoked_at=COALESCE(revoked_at,sqlc.arg(now)),encrypted_secret=NULL
WHERE user_id=sqlc.arg(user_id)
AND group_id IN (SELECT id FROM account_groups WHERE tenant_id=sqlc.arg(tenant_id));
