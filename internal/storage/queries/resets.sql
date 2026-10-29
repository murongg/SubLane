-- name: GetResetAccount :one
SELECT id,tenant_id,provider,enabled,status,models_revision,reset_config,reset_revision,reset_next_run_at,reset_time_zone
FROM accounts WHERE id=sqlc.arg(id) AND tenant_id=sqlc.arg(tenant_id);

-- name: SaveResetConfig :execrows
UPDATE accounts SET reset_config=sqlc.arg(config),reset_revision=reset_revision+1,
reset_next_run_at=sqlc.arg(next_at),reset_time_zone=sqlc.arg(zone),reset_check_at=0
WHERE id=sqlc.arg(id) AND tenant_id=sqlc.arg(tenant_id) AND reset_revision=sqlc.arg(revision);

-- name: AdvanceResetSchedule :execrows
UPDATE accounts SET reset_next_run_at=sqlc.arg(next_at),reset_time_zone=sqlc.arg(zone)
WHERE id=sqlc.arg(id) AND tenant_id=sqlc.arg(tenant_id) AND reset_revision=sqlc.arg(revision)
AND reset_next_run_at=sqlc.arg(previous_at);

-- name: CreateResetRun :execrows
INSERT OR IGNORE INTO reset_runs(request_id,account_id,trigger_kind,occurrence,outcome,reason,started_at,updated_at)
VALUES(sqlc.arg(request_id),sqlc.arg(account_id),sqlc.arg(trigger_kind),sqlc.narg(occurrence),sqlc.arg(outcome),sqlc.arg(reason),sqlc.arg(now),sqlc.arg(now));

-- name: PendingResetRun :one
SELECT * FROM reset_runs WHERE account_id=sqlc.arg(account_id) AND outcome='pending' LIMIT 1;

-- name: FinishResetRun :execrows
UPDATE reset_runs SET outcome=sqlc.arg(outcome),reason=sqlc.arg(reason),updated_at=sqlc.arg(now)
WHERE request_id=sqlc.arg(request_id) AND outcome='pending';

-- name: CountResetUsage :one
SELECT count(*) FROM reset_runs WHERE account_id=sqlc.arg(account_id) AND outcome IN ('pending','success')
AND started_at>=sqlc.arg(day_start) AND started_at<sqlc.arg(day_end);

-- name: ListResetRuns :many
SELECT * FROM reset_runs WHERE account_id=sqlc.arg(account_id) ORDER BY id DESC LIMIT 20;

-- name: PruneResetRuns :exec
DELETE FROM reset_runs WHERE reset_runs.account_id=sqlc.arg(account_id) AND reset_runs.outcome!='pending'
AND reset_runs.started_at<sqlc.arg(day_start) AND reset_runs.id NOT IN (SELECT history.id FROM reset_runs history WHERE history.account_id=sqlc.arg(account_id) ORDER BY history.id DESC LIMIT 100);

-- name: NextResetCheck :one
SELECT id,tenant_id,reset_check_at FROM accounts WHERE provider='codex' AND reset_check_at<=sqlc.arg(now)
AND EXISTS(SELECT 1 FROM tenants WHERE tenants.id=accounts.tenant_id AND tenants.status='active')
AND (json_extract(reset_config,'$.exhausted')=1 OR reset_next_run_at>0)
AND NOT EXISTS(SELECT 1 FROM reset_runs WHERE account_id=accounts.id AND outcome='pending')
ORDER BY reset_check_at,id LIMIT 1;

-- name: ClaimResetCheck :execrows
UPDATE accounts SET reset_check_at=sqlc.arg(next_at) WHERE id=sqlc.arg(id) AND reset_check_at=sqlc.arg(previous_at);

-- name: InvalidateAccountUsage :exec
UPDATE accounts SET usage_revision=usage_revision+1 WHERE id=sqlc.arg(id);
