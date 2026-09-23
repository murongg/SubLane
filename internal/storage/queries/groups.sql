-- name: ListGroups :many
SELECT g.*, (SELECT count(*) FROM group_accounts a WHERE a.group_id=g.id) AS account_count,
(SELECT count(*) FROM group_members grant_row
 JOIN effective_group_access access ON access.group_id=grant_row.group_id AND access.user_id=grant_row.user_id
 WHERE grant_row.group_id=g.id) AS member_count
FROM account_groups g WHERE g.tenant_id=sqlc.arg(tenant_id) ORDER BY g.id;

-- name: GetGroup :one
SELECT * FROM account_groups WHERE id=sqlc.arg(id);

-- name: GetTenantGroup :one
SELECT * FROM account_groups WHERE id=sqlc.arg(id) AND tenant_id=sqlc.arg(tenant_id);

-- name: CountGroups :one
SELECT count(*) FROM account_groups WHERE tenant_id=sqlc.arg(tenant_id);

-- name: CreateGroup :execlastid
INSERT INTO account_groups(tenant_id,name,enabled,created_at,updated_at)
VALUES(sqlc.arg(tenant_id),sqlc.arg(name),sqlc.arg(enabled),sqlc.arg(now),sqlc.arg(now));

-- name: FindGroupName :one
SELECT id FROM account_groups WHERE tenant_id=sqlc.arg(tenant_id) AND name=sqlc.arg(name) COLLATE NOCASE;

-- name: UpdateGroup :exec
UPDATE account_groups SET name=sqlc.arg(name),enabled=sqlc.arg(enabled),updated_at=sqlc.arg(now)
WHERE id=sqlc.arg(id) AND tenant_id=sqlc.arg(tenant_id);

-- name: ListGroupAccounts :many
SELECT account_id FROM group_accounts WHERE group_id=sqlc.arg(group_id) ORDER BY account_id;

-- name: ClearGroupAccounts :exec
DELETE FROM group_accounts WHERE group_id=sqlc.arg(group_id);

-- name: AddGroupAccount :exec
INSERT INTO group_accounts(group_id,account_id) VALUES(sqlc.arg(group_id),sqlc.arg(account_id));

-- name: GroupAccountExists :one
SELECT EXISTS(SELECT 1 FROM accounts WHERE id=sqlc.arg(account_id) AND tenant_id=sqlc.arg(tenant_id));

-- name: ListMemberGroups :many
SELECT m.group_id FROM group_members m JOIN account_groups g ON g.id=m.group_id
WHERE m.user_id=sqlc.arg(user_id) AND g.tenant_id=sqlc.arg(tenant_id) ORDER BY m.group_id;

-- name: ClearMemberGroups :exec
DELETE FROM group_members WHERE user_id=sqlc.arg(user_id)
AND group_id IN (SELECT id FROM account_groups WHERE tenant_id=sqlc.arg(tenant_id));

-- name: MemberExistsInTenant :one
SELECT EXISTS(SELECT 1 FROM memberships m
WHERE m.tenant_id=sqlc.arg(tenant_id) AND m.user_id=sqlc.arg(user_id));

-- name: GrantMemberGroup :exec
INSERT INTO group_members(group_id,user_id) VALUES(sqlc.arg(group_id),sqlc.arg(user_id));

-- name: CanUseGroup :one
SELECT EXISTS(SELECT 1 FROM account_groups g JOIN users u ON u.id=sqlc.arg(user_id)
WHERE g.id=sqlc.arg(group_id) AND g.enabled=1 AND u.enabled=1
AND EXISTS(SELECT 1 FROM effective_group_access access WHERE access.group_id=g.id AND access.user_id=u.id));

-- name: ListAvailableGroups :many
SELECT g.id,g.name FROM account_groups g JOIN users u ON u.id=sqlc.arg(user_id)
WHERE g.tenant_id=sqlc.arg(tenant_id) AND g.enabled=1 AND u.enabled=1
AND EXISTS(SELECT 1 FROM effective_group_access access WHERE access.group_id=g.id AND access.user_id=u.id) ORDER BY g.id;

-- name: GroupConnectionStatus :one
SELECT CASE WHEN EXISTS(
 SELECT 1 FROM group_accounts ga JOIN accounts a ON a.id=ga.account_id JOIN account_groups g ON g.id=ga.group_id JOIN users u ON u.id=sqlc.arg(user_id)
 WHERE g.tenant_id=sqlc.arg(tenant_id) AND g.enabled=1 AND a.enabled=1 AND a.status='ready' AND u.enabled=1 AND EXISTS(SELECT 1 FROM effective_group_access access WHERE access.group_id=g.id AND access.user_id=u.id)
) THEN 'ready' WHEN EXISTS(
 SELECT 1 FROM group_accounts ga JOIN accounts a ON a.id=ga.account_id JOIN account_groups g ON g.id=ga.group_id JOIN users u ON u.id=sqlc.arg(user_id)
 WHERE g.tenant_id=sqlc.arg(tenant_id) AND g.enabled=1 AND a.enabled=1 AND u.enabled=1 AND EXISTS(SELECT 1 FROM effective_group_access access WHERE access.group_id=g.id AND access.user_id=u.id)
) THEN 'needs_attention' ELSE 'not_configured' END AS status;

-- name: CountGroupMembers :one
SELECT count(*) FROM group_members grant_row
JOIN effective_group_access access ON access.group_id=grant_row.group_id AND access.user_id=grant_row.user_id
WHERE grant_row.group_id=sqlc.arg(group_id);

-- name: ListPoolMembers :many
SELECT u.id,u.username FROM group_members grant_row
JOIN effective_group_access access ON access.group_id=grant_row.group_id AND access.user_id=grant_row.user_id
JOIN users u ON u.id=access.user_id
WHERE grant_row.group_id=sqlc.arg(group_id)
ORDER BY u.username,u.id LIMIT 100;

-- name: SetGroupModelPolicy :exec
UPDATE account_groups SET restricted_models=sqlc.arg(restricted) WHERE id=sqlc.arg(id);

-- name: ClearGroupModels :exec
DELETE FROM group_models WHERE group_id=sqlc.arg(group_id);

-- name: AddGroupModel :exec
INSERT INTO group_models(group_id,model) VALUES(sqlc.arg(group_id),sqlc.arg(model));

-- name: ListGroupModels :many
SELECT model FROM group_models WHERE group_id=sqlc.arg(group_id) ORDER BY model;
