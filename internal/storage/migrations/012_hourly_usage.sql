CREATE TABLE usage_hourly_coverage (
 id INTEGER PRIMARY KEY CHECK(id=1),
 started_at INTEGER NOT NULL
);
INSERT INTO usage_hourly_coverage(id,started_at) VALUES(1,CAST(strftime('%s','now') AS INTEGER));
CREATE TABLE usage_hourly (
 hour INTEGER NOT NULL,
 user_id INTEGER NOT NULL,
 requests INTEGER NOT NULL CHECK(requests>=0),
 input_tokens INTEGER NOT NULL CHECK(input_tokens>=0),
 output_tokens INTEGER NOT NULL CHECK(output_tokens>=0),
 input_reported INTEGER NOT NULL CHECK(input_reported BETWEEN 0 AND requests),
 output_reported INTEGER NOT NULL CHECK(output_reported BETWEEN 0 AND requests),
 PRIMARY KEY(hour,user_id)
);
CREATE INDEX usage_hourly_user ON usage_hourly(user_id,hour);
