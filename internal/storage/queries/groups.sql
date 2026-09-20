-- name: ListGroups :many
SELECT g.*, (SELECT count(*) FROM group_accounts a WHERE a.group_id=g.id) AS account_count,
(SELECT count(*) FROM group_members m JOIN users u ON u.id=m.user_id WHERE m.group_id=g.id AND u.role='member') AS member_count
FROM account_groups g ORDER BY g.id;

-- name: GetGroup :one
SELECT * FROM account_groups WHERE id=sqlc.arg(id);

-- name: CountGroups :one
SELECT count(*) FROM account_groups;

-- name: CreateGroup :execlastid
INSERT INTO account_groups(name,enabled,created_at,updated_at)
VALUES(sqlc.arg(name),sqlc.arg(enabled),sqlc.arg(now),sqlc.arg(now));

-- name: FindGroupName :one
SELECT id FROM account_groups WHERE name=sqlc.arg(name) COLLATE NOCASE;

-- name: UpdateGroup :exec
UPDATE account_groups SET name=sqlc.arg(name),enabled=sqlc.arg(enabled),updated_at=sqlc.arg(now) WHERE id=sqlc.arg(id);

-- name: ListGroupAccounts :many
SELECT account_id FROM group_accounts WHERE group_id=sqlc.arg(group_id) ORDER BY account_id;

-- name: ClearGroupAccounts :exec
DELETE FROM group_accounts WHERE group_id=sqlc.arg(group_id);

-- name: AddGroupAccount :exec
INSERT INTO group_accounts(group_id,account_id) VALUES(sqlc.arg(group_id),sqlc.arg(account_id));

-- name: AddDefaultGroupAccount :exec
INSERT INTO group_accounts(group_id,account_id) VALUES(1,sqlc.arg(account_id));

-- name: GroupAccountExists :one
SELECT EXISTS(SELECT 1 FROM accounts WHERE id=sqlc.arg(account_id));

-- name: AddDefaultGroupMember :exec
INSERT INTO group_members(group_id,user_id) VALUES(1,sqlc.arg(user_id));

-- name: ListMemberGroups :many
SELECT group_id FROM group_members WHERE user_id=sqlc.arg(user_id) ORDER BY group_id;

-- name: ClearMemberGroups :exec
DELETE FROM group_members WHERE user_id=sqlc.arg(user_id);

-- name: GrantMemberGroup :exec
INSERT INTO group_members(group_id,user_id) VALUES(sqlc.arg(group_id),sqlc.arg(user_id));

-- name: CanUseGroup :one
SELECT EXISTS(SELECT 1 FROM account_groups g JOIN users u ON u.id=sqlc.arg(user_id)
WHERE g.id=sqlc.arg(group_id) AND g.enabled=1 AND u.enabled=1
AND (u.role='admin' OR EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=g.id AND m.user_id=u.id)));

-- name: ListAvailableGroups :many
SELECT g.id,g.name FROM account_groups g JOIN users u ON u.id=sqlc.arg(user_id)
WHERE g.enabled=1 AND u.enabled=1
AND (u.role='admin' OR EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=g.id AND m.user_id=u.id)) ORDER BY g.id;

-- name: GroupConnectionStatus :one
SELECT CASE WHEN EXISTS(
 SELECT 1 FROM group_accounts ga JOIN accounts a ON a.id=ga.account_id JOIN account_groups g ON g.id=ga.group_id JOIN users u ON u.id=sqlc.arg(user_id)
 WHERE g.enabled=1 AND a.enabled=1 AND a.status='ready' AND u.enabled=1 AND (u.role='admin' OR EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=g.id AND m.user_id=u.id))
) THEN 'ready' WHEN EXISTS(
 SELECT 1 FROM group_accounts ga JOIN accounts a ON a.id=ga.account_id JOIN account_groups g ON g.id=ga.group_id JOIN users u ON u.id=sqlc.arg(user_id)
 WHERE g.enabled=1 AND a.enabled=1 AND u.enabled=1 AND (u.role='admin' OR EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=g.id AND m.user_id=u.id))
) THEN 'needs_attention' ELSE 'not_configured' END AS status;

-- name: CountGroupMembers :one
SELECT count(*) FROM group_members m JOIN users u ON u.id=m.user_id WHERE m.group_id=sqlc.arg(group_id) AND u.role='member';

-- name: SetGroupModelPolicy :exec
UPDATE account_groups SET restricted_models=sqlc.arg(restricted) WHERE id=sqlc.arg(id);

-- name: ClearGroupModels :exec
DELETE FROM group_models WHERE group_id=sqlc.arg(group_id);

-- name: AddGroupModel :exec
INSERT INTO group_models(group_id,model) VALUES(sqlc.arg(group_id),sqlc.arg(model));

-- name: ListGroupModels :many
SELECT model FROM group_models WHERE group_id=sqlc.arg(group_id) ORDER BY model;
