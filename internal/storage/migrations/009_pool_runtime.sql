ALTER TABLE accounts ADD COLUMN max_concurrency INTEGER NOT NULL DEFAULT 2 CHECK(max_concurrency BETWEEN 1 AND 8);
CREATE TABLE account_runtime (
 account_id TEXT PRIMARY KEY NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 cooldown_until INTEGER NOT NULL DEFAULT 0,
 reason TEXT NOT NULL DEFAULT '',
 failures INTEGER NOT NULL DEFAULT 0,
 last_failure_at INTEGER NOT NULL DEFAULT 0,
 revision INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE request_records (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 user_id INTEGER NOT NULL,
 key_id INTEGER NOT NULL,
 group_id INTEGER NOT NULL,
 account_id TEXT NOT NULL DEFAULT '',
 provider TEXT NOT NULL DEFAULT '',
 model TEXT NOT NULL DEFAULT '' CHECK(length(model)<=160),
 transport TEXT NOT NULL CHECK(transport IN ('http','websocket')),
 operation TEXT NOT NULL CHECK(operation IN ('responses','chat','compact')),
 started_at INTEGER NOT NULL,
 duration_ms INTEGER NOT NULL CHECK(duration_ms>=0),
 outcome TEXT NOT NULL CHECK(outcome IN ('success','incomplete','error','canceled','rejected')),
 error_code TEXT NOT NULL DEFAULT '' CHECK(length(error_code)<=64),
 upstream_status INTEGER,
 input_tokens INTEGER,
 output_tokens INTEGER,
 cached_tokens INTEGER
);
CREATE INDEX request_records_time ON request_records(started_at);
CREATE INDEX request_records_account ON request_records(account_id,id);
