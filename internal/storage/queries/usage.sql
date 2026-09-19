-- name: GetAccountUsage :one
SELECT snapshot, updated_at FROM account_usage WHERE account_id = sqlc.arg(account_id);

-- name: SaveAccountUsage :exec
INSERT INTO account_usage(account_id, snapshot, updated_at)
VALUES (sqlc.arg(account_id), sqlc.arg(snapshot), sqlc.arg(updated_at))
ON CONFLICT(account_id) DO UPDATE SET snapshot = excluded.snapshot, updated_at = excluded.updated_at
WHERE excluded.updated_at >= account_usage.updated_at;
