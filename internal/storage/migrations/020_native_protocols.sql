-- SQLite cannot extend a CHECK constraint in place. Preserve history and its AUTOINCREMENT high-water mark.
CREATE TABLE request_records_native (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 user_id INTEGER NOT NULL,
 key_id INTEGER NOT NULL,
 group_id INTEGER NOT NULL,
 account_id TEXT NOT NULL DEFAULT '',
 provider TEXT NOT NULL DEFAULT '',
 model TEXT NOT NULL DEFAULT '' CHECK(length(model)<=160),
 transport TEXT NOT NULL CHECK(transport IN ('http','websocket')),
 operation TEXT NOT NULL CHECK(operation IN ('responses','chat','compact','messages','gemini')),
 started_at INTEGER NOT NULL,
 duration_ms INTEGER NOT NULL CHECK(duration_ms>=0),
 outcome TEXT NOT NULL CHECK(outcome IN ('success','incomplete','error','canceled','rejected')),
 error_code TEXT NOT NULL DEFAULT '' CHECK(length(error_code)<=64),
 upstream_status INTEGER,
 input_tokens INTEGER,
 output_tokens INTEGER,
 cached_tokens INTEGER,
 request_id TEXT NOT NULL DEFAULT '' CHECK(length(request_id)<=64),
 first_token_ms INTEGER CHECK(first_token_ms>=0)
);
INSERT INTO request_records_native SELECT * FROM request_records;
INSERT INTO sqlite_sequence(name,seq)
 SELECT 'request_records_native', seq FROM sqlite_sequence WHERE name='request_records'
 AND NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name='request_records_native');
UPDATE sqlite_sequence SET seq=MAX(seq,COALESCE((SELECT seq FROM sqlite_sequence WHERE name='request_records'),0))
 WHERE name='request_records_native';
DROP TABLE request_records;
ALTER TABLE request_records_native RENAME TO request_records;
CREATE INDEX request_records_time ON request_records(started_at);
CREATE INDEX request_records_account ON request_records(account_id,id);
CREATE INDEX request_records_request_id ON request_records(request_id) WHERE request_id!='';
