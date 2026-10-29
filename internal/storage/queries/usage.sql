-- name: GetAccountUsage :one
SELECT u.snapshot, u.updated_at, a.models_revision+a.usage_revision AS revision
FROM accounts a LEFT JOIN account_usage u ON u.account_id = a.id AND u.revision = a.models_revision+a.usage_revision
WHERE a.id = sqlc.arg(account_id);

-- name: SaveAccountUsage :execrows
INSERT INTO account_usage(account_id, snapshot, updated_at, revision)
SELECT id, sqlc.arg(snapshot), sqlc.arg(updated_at), models_revision+usage_revision FROM accounts
WHERE id = sqlc.arg(account_id) AND models_revision+usage_revision = sqlc.arg(revision)
AND enabled = 1 AND status != 'reauth_required'
ON CONFLICT(account_id) DO UPDATE SET snapshot = excluded.snapshot, updated_at = excluded.updated_at, revision = excluded.revision
WHERE excluded.revision != account_usage.revision OR excluded.updated_at >= account_usage.updated_at;

-- name: ListGroupUsage :many
SELECT a.id, a.provider, a.models_revision+a.usage_revision AS revision, u.snapshot, u.updated_at
FROM accounts a JOIN group_accounts ga ON ga.account_id=a.id
LEFT JOIN account_usage u ON u.account_id=a.id AND u.revision=a.models_revision+a.usage_revision
WHERE ga.group_id=sqlc.arg(group_id) AND a.enabled=1 AND a.status!='reauth_required';

-- name: ListWorkspaceUsage :many
SELECT a.id, a.provider, a.models_revision+a.usage_revision AS revision, u.snapshot, u.updated_at
FROM accounts a LEFT JOIN account_usage u ON u.account_id=a.id AND u.revision=a.models_revision+a.usage_revision
WHERE a.tenant_id=sqlc.arg(tenant_id);
