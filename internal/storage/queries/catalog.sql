-- name: GetAccountCatalog :one
SELECT models_snapshot, models_revision FROM accounts WHERE id = sqlc.arg(id);

-- name: SaveAccountCatalog :execrows
UPDATE accounts SET models_snapshot = sqlc.arg(snapshot)
WHERE id = sqlc.arg(id) AND models_revision = sqlc.arg(revision)
AND enabled = 1 AND status != 'reauth_required'
AND COALESCE(json_extract(models_snapshot, '$.updated_at'), 0) <= CAST(sqlc.arg(observed_at) AS INTEGER);

-- name: InvalidateAccountCatalog :exec
UPDATE accounts SET models_snapshot = NULL, models_revision = models_revision + 1
WHERE id = sqlc.arg(id);

-- name: ListGroupCatalogs :many
SELECT a.id, a.provider, a.models_snapshot, a.models_revision
FROM accounts a JOIN group_accounts ga ON ga.account_id = a.id
WHERE ga.group_id = sqlc.arg(group_id) AND a.enabled = 1 AND a.status != 'reauth_required'
ORDER BY a.id;
