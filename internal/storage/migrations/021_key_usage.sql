-- Legacy aggregates cannot be attributed to individual keys; keep them in the all-key bucket.

CREATE TABLE usage_daily_next (
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
 key_id INTEGER NOT NULL DEFAULT 0 CHECK(key_id>=0),
 PRIMARY KEY(day,user_id,group_id,provider,model,key_id)
);

INSERT INTO usage_daily_next(day,user_id,group_id,provider,model,requests,completed,incomplete,errors,canceled,rejected,duration_ms,input_tokens,output_tokens,cached_tokens,input_reported,output_reported,cached_reported)
SELECT day,user_id,group_id,provider,model,requests,completed,incomplete,errors,canceled,rejected,duration_ms,input_tokens,output_tokens,cached_tokens,input_reported,output_reported,cached_reported FROM usage_daily;

DROP TABLE usage_daily;

ALTER TABLE usage_daily_next RENAME TO usage_daily;

CREATE TABLE usage_hourly_next (
 tenant_id INTEGER NOT NULL REFERENCES tenants(id),
 hour INTEGER NOT NULL,
 user_id INTEGER NOT NULL,
 requests INTEGER NOT NULL CHECK(requests>=0),
 input_tokens INTEGER NOT NULL CHECK(input_tokens>=0),
 output_tokens INTEGER NOT NULL CHECK(output_tokens>=0),
 input_reported INTEGER NOT NULL CHECK(input_reported BETWEEN 0 AND requests),
 output_reported INTEGER NOT NULL CHECK(output_reported BETWEEN 0 AND requests),
 key_id INTEGER NOT NULL DEFAULT 0 CHECK(key_id>=0),
 PRIMARY KEY(tenant_id,hour,user_id,key_id)
);

INSERT INTO usage_hourly_next(tenant_id,hour,user_id,requests,input_tokens,output_tokens,input_reported,output_reported)
SELECT tenant_id,hour,user_id,requests,input_tokens,output_tokens,input_reported,output_reported FROM usage_hourly;

DROP TABLE usage_hourly;

ALTER TABLE usage_hourly_next RENAME TO usage_hourly;

CREATE INDEX usage_daily_user ON usage_daily(user_id,day);

CREATE INDEX usage_hourly_user ON usage_hourly(tenant_id,user_id,hour);

CREATE INDEX usage_hourly_retention ON usage_hourly(hour);

INSERT INTO settings(key,value) VALUES('usage.keys.started_at',CAST(unixepoch() AS TEXT));
