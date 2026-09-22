-- name: ListTokenBudgets :many
SELECT b.*, COALESCE(g.name,'') AS group_name FROM token_budgets b
LEFT JOIN account_groups g ON g.id=b.group_id WHERE b.user_id=? ORDER BY b.id;

-- name: SaveTokenBudget :one
INSERT INTO token_budgets(user_id,group_id,model,period,token_limit,enabled,created_at)
VALUES(?,?,?,?,?,?,?) ON CONFLICT(user_id,group_id,model,period)
DO UPDATE SET token_limit=excluded.token_limit,enabled=excluded.enabled RETURNING id;

-- name: GetTokenBudgetUsage :one
SELECT used FROM token_budget_usage WHERE budget_id=? AND window_start=?;

-- name: CountTokenBudgetPending :one
SELECT count(*) FROM token_budget_entries WHERE budget_id=? AND state='pending';

-- name: BeginTokenBudgetUsage :exec
INSERT INTO token_budget_usage(budget_id,window_start,window_end) VALUES(?,?,?) ON CONFLICT DO NOTHING;

-- name: BeginTokenBudgetEntry :exec
INSERT INTO token_budget_entries(request_id,budget_id,window_start,started_at,state) VALUES(?,?,?,?,'active');

-- name: ListTokenBudgetEntries :many
SELECT e.* FROM token_budget_entries e JOIN token_budgets b ON b.id=e.budget_id
WHERE e.request_id=? AND b.user_id=? ORDER BY e.budget_id;

-- name: AddTokenBudgetUsage :exec
UPDATE token_budget_usage SET used=used+sqlc.arg(tokens) WHERE budget_id=sqlc.arg(budget_id) AND window_start=sqlc.arg(window_start);

-- name: SetTokenBudgetEntry :exec
UPDATE token_budget_entries SET state=?,tokens=?,manual=? WHERE request_id=? AND budget_id=?;

-- name: RecoverTokenBudgetEntries :exec
UPDATE token_budget_entries SET state='pending' WHERE state='active';

-- name: ListPendingTokenRequests :many
SELECT e.request_id, CAST(min(e.started_at) AS INTEGER) AS started_at, CAST(max(e.tokens) AS INTEGER) AS known_tokens
FROM token_budget_entries e JOIN token_budgets b ON b.id=e.budget_id
WHERE b.user_id=? AND e.state='pending' GROUP BY e.request_id ORDER BY started_at LIMIT 256;

-- name: PruneTokenBudgetEntries :exec
DELETE FROM token_budget_entries WHERE state='settled' AND started_at<sqlc.arg(before_time);

-- name: PruneTokenBudgetUsage :exec
DELETE FROM token_budget_usage WHERE window_end<sqlc.arg(before_time)
AND NOT EXISTS(SELECT 1 FROM token_budget_entries e WHERE e.budget_id=token_budget_usage.budget_id AND e.window_start=token_budget_usage.window_start);
