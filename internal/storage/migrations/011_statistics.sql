CREATE TABLE usage_coverage (
 id INTEGER PRIMARY KEY CHECK(id=1),
 started_at INTEGER NOT NULL
);
INSERT INTO usage_coverage(id,started_at) VALUES(1,CAST(strftime('%s','now') AS INTEGER));
CREATE TABLE usage_daily (
 day INTEGER NOT NULL,
 user_id INTEGER NOT NULL,
 group_id INTEGER NOT NULL,
 provider TEXT NOT NULL,
 model TEXT NOT NULL CHECK(length(model)<=180),
 requests INTEGER NOT NULL DEFAULT 0 CHECK(requests>=0),
 completed INTEGER NOT NULL DEFAULT 0 CHECK(completed>=0),
 incomplete INTEGER NOT NULL DEFAULT 0 CHECK(incomplete>=0),
 errors INTEGER NOT NULL DEFAULT 0 CHECK(errors>=0),
 canceled INTEGER NOT NULL DEFAULT 0 CHECK(canceled>=0),
 rejected INTEGER NOT NULL DEFAULT 0 CHECK(rejected>=0),
 duration_ms INTEGER NOT NULL DEFAULT 0 CHECK(duration_ms>=0),
 input_tokens INTEGER NOT NULL DEFAULT 0 CHECK(input_tokens>=0),
 output_tokens INTEGER NOT NULL DEFAULT 0 CHECK(output_tokens>=0),
 cached_tokens INTEGER NOT NULL DEFAULT 0 CHECK(cached_tokens>=0),
 input_reported INTEGER NOT NULL DEFAULT 0 CHECK(input_reported>=0),
 output_reported INTEGER NOT NULL DEFAULT 0 CHECK(output_reported>=0),
 cached_reported INTEGER NOT NULL DEFAULT 0 CHECK(cached_reported>=0),
 PRIMARY KEY(day,user_id,group_id,provider,model)
);
CREATE INDEX usage_daily_user ON usage_daily(user_id,day);
