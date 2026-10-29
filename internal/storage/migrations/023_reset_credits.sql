ALTER TABLE accounts ADD COLUMN reset_config TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(reset_config) AND length(reset_config)<=2048);
ALTER TABLE accounts ADD COLUMN reset_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE accounts ADD COLUMN reset_next_run_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE accounts ADD COLUMN reset_time_zone TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN reset_check_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE accounts ADD COLUMN usage_revision INTEGER NOT NULL DEFAULT 0;

CREATE TABLE reset_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    trigger_kind TEXT NOT NULL CHECK(trigger_kind IN ('scheduled','exhausted','manual')),
    occurrence INTEGER,
    outcome TEXT NOT NULL CHECK(outcome IN ('pending','success','skipped','failed')),
    reason TEXT NOT NULL CHECK(length(reason)<=64),
    started_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(account_id,occurrence)
);
CREATE UNIQUE INDEX reset_pending ON reset_runs(account_id) WHERE outcome='pending';
CREATE INDEX reset_history ON reset_runs(account_id,id DESC);
CREATE INDEX reset_checks ON accounts(reset_check_at) WHERE provider='codex';
