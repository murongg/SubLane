CREATE TABLE token_budgets (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 group_id INTEGER NOT NULL DEFAULT 0 CHECK(group_id>=0),
 model TEXT NOT NULL DEFAULT '' CHECK(length(model)<=128),
 period TEXT NOT NULL CHECK(period IN ('day','month')),
 token_limit INTEGER NOT NULL CHECK(token_limit BETWEEN 1 AND 1000000000000),
 enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),
 created_at INTEGER NOT NULL,
 UNIQUE(user_id,group_id,model,period)
);
CREATE TABLE token_budget_usage (
 budget_id INTEGER NOT NULL REFERENCES token_budgets(id) ON DELETE CASCADE,
 window_start INTEGER NOT NULL,
 window_end INTEGER NOT NULL,
 used INTEGER NOT NULL DEFAULT 0 CHECK(used>=0),
 PRIMARY KEY(budget_id,window_start)
);
CREATE TABLE token_budget_entries (
 request_id TEXT NOT NULL CHECK(length(request_id)<=64),
 budget_id INTEGER NOT NULL REFERENCES token_budgets(id) ON DELETE CASCADE,
 window_start INTEGER NOT NULL,
 started_at INTEGER NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('active','pending','settled')),
 tokens INTEGER NOT NULL DEFAULT 0 CHECK(tokens BETWEEN 0 AND 2000000000),
 manual INTEGER NOT NULL DEFAULT 0 CHECK(manual IN (0,1)),
 PRIMARY KEY(request_id,budget_id)
);
CREATE INDEX token_budget_entries_budget ON token_budget_entries(budget_id,state);
