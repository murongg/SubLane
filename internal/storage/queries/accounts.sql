-- name: CountAccounts :one
SELECT count(*) FROM accounts;

-- name: ListAccounts :many
SELECT id, provider, name, email, plan, enabled, status, expires_at, created_at, updated_at
FROM accounts ORDER BY created_at DESC, id DESC LIMIT 100;

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = sqlc.arg(id);

-- name: CreateAccount :execrows
INSERT INTO accounts(id, provider, name, account_id, email, plan, enabled, status, credential, expires_at, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(provider), sqlc.arg(name), sqlc.arg(account_id), sqlc.arg(email), sqlc.arg(plan), 1, sqlc.arg(status), sqlc.arg(credential), sqlc.arg(expires_at), sqlc.arg(created_at), sqlc.arg(updated_at))
ON CONFLICT(provider, account_id) DO NOTHING;

-- name: UpdateAccountCredential :exec
UPDATE accounts SET credential = sqlc.arg(credential), email = sqlc.arg(email), plan = sqlc.arg(plan), status = sqlc.arg(status), expires_at = sqlc.arg(expires_at), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: SetAccountEnabled :exec
UPDATE accounts SET enabled = sqlc.arg(enabled), updated_at = sqlc.arg(updated_at) WHERE id = sqlc.arg(id);

-- name: SetAccountStatus :exec
UPDATE accounts SET status = sqlc.arg(status), updated_at = sqlc.arg(updated_at) WHERE id = sqlc.arg(id);

-- name: DeleteAccount :execrows
DELETE FROM accounts WHERE id = sqlc.arg(id);
