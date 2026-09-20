-- name: GetAccountUsage :one
SELECT u.snapshot, u.updated_at, a.models_revision AS revision
FROM accounts a LEFT JOIN account_usage u ON u.account_id = a.id AND u.revision = a.models_revision
WHERE a.id = sqlc.arg(account_id);

-- name: SaveAccountUsage :execrows
INSERT INTO account_usage(account_id, snapshot, updated_at, revision)
SELECT id, sqlc.arg(snapshot), sqlc.arg(updated_at), models_revision FROM accounts
WHERE id = sqlc.arg(account_id) AND models_revision = sqlc.arg(revision)
AND enabled = 1 AND status != 'reauth_required'
ON CONFLICT(account_id) DO UPDATE SET snapshot = excluded.snapshot, updated_at = excluded.updated_at, revision = excluded.revision
WHERE excluded.revision != account_usage.revision OR excluded.updated_at >= account_usage.updated_at;
