CREATE TABLE allocation_teams (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 name TEXT NOT NULL COLLATE NOCASE UNIQUE CHECK(length(name) BETWEEN 1 AND 64),
 enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),
 created_at INTEGER NOT NULL
);
CREATE TABLE allocation_team_members (
 team_id INTEGER NOT NULL REFERENCES allocation_teams(id),
 user_id INTEGER NOT NULL REFERENCES users(id),
 PRIMARY KEY(team_id,user_id)
);
CREATE TABLE allocation_schemes (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 64),
 team_id INTEGER NOT NULL REFERENCES allocation_teams(id),
 group_id INTEGER NOT NULL UNIQUE REFERENCES account_groups(id),
 enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),
 created_at INTEGER NOT NULL
);
CREATE TABLE allocation_revisions (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 scheme_id INTEGER NOT NULL REFERENCES allocation_schemes(id),
 effective_at INTEGER NOT NULL,
 config TEXT NOT NULL CHECK(json_valid(config)),
 UNIQUE(scheme_id,effective_at)
);
CREATE TABLE allocation_keys (
 key_id INTEGER PRIMARY KEY REFERENCES api_keys(id),
 scheme_id INTEGER NOT NULL REFERENCES allocation_schemes(id)
);
CREATE TABLE allocation_entries (
 request_id TEXT PRIMARY KEY,
 scheme_id INTEGER NOT NULL REFERENCES allocation_schemes(id),
 revision_id INTEGER NOT NULL REFERENCES allocation_revisions(id),
 user_id INTEGER NOT NULL REFERENCES users(id),
 account_id TEXT NOT NULL,
 model TEXT NOT NULL,
 mode TEXT NOT NULL CHECK(mode IN ('tokens','amount','ratio')),
 window_start INTEGER NOT NULL,
 reset_at INTEGER NOT NULL,
 started_at INTEGER NOT NULL,
 finished_at INTEGER NOT NULL DEFAULT 0,
 state TEXT NOT NULL CHECK(state IN ('active','pending','observed','settled')),
 input_tokens INTEGER NOT NULL DEFAULT 0,
 output_tokens INTEGER NOT NULL DEFAULT 0,
 cached_tokens INTEGER NOT NULL DEFAULT 0,
 cost INTEGER NOT NULL DEFAULT 0 CHECK(cost>=0),
 manual INTEGER NOT NULL DEFAULT 0 CHECK(manual IN (0,1))
);
CREATE INDEX allocation_entries_member ON allocation_entries(scheme_id,user_id,window_start);
CREATE INDEX allocation_entries_pending ON allocation_entries(account_id,state);
CREATE TABLE allocation_windows (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 scheme_id INTEGER NOT NULL REFERENCES allocation_schemes(id),
 account_id TEXT NOT NULL,
 kind TEXT NOT NULL,
 reset_at INTEGER NOT NULL,
 account_revision INTEGER NOT NULL,
 observed_at INTEGER NOT NULL,
 observed_points INTEGER NOT NULL,
 baseline_points INTEGER NOT NULL,
 unassigned INTEGER NOT NULL DEFAULT 0,
 UNIQUE(scheme_id,account_id,kind,reset_at)
);
CREATE TABLE allocation_window_members (
 window_id INTEGER NOT NULL REFERENCES allocation_windows(id) ON DELETE CASCADE,
 user_id INTEGER NOT NULL REFERENCES users(id),
 allowance INTEGER NOT NULL CHECK(allowance>=0),
 PRIMARY KEY(window_id,user_id)
);
CREATE TABLE allocation_debits (
 request_id TEXT NOT NULL REFERENCES allocation_entries(request_id) ON DELETE CASCADE,
 window_id INTEGER NOT NULL REFERENCES allocation_windows(id) ON DELETE CASCADE,
 points INTEGER NOT NULL DEFAULT 0 CHECK(points>=0),
 reconciled INTEGER NOT NULL DEFAULT 0 CHECK(reconciled IN (0,1)),
 PRIMARY KEY(request_id,window_id)
);
-- A managed pool reserves physical accounts, including while paused. Removing the
-- pool grant must not make the same subscription available through an unbound key.
CREATE TRIGGER allocation_exclusive_account BEFORE INSERT ON group_accounts
WHEN EXISTS(SELECT 1 FROM allocation_schemes WHERE group_id=NEW.group_id) OR EXISTS (
 SELECT 1 FROM allocation_schemes s JOIN group_accounts ga ON ga.group_id=s.group_id
 WHERE ga.account_id=NEW.account_id AND ga.group_id<>NEW.group_id
)
BEGIN SELECT RAISE(ABORT,'allocation_pool_conflict'); END;
CREATE TRIGGER allocation_fixed_pool BEFORE DELETE ON group_accounts
WHEN EXISTS(SELECT 1 FROM allocation_schemes WHERE group_id=OLD.group_id)
BEGIN SELECT RAISE(ABORT,'allocation_pool_locked'); END;

CREATE INDEX allocation_windows_reset ON allocation_windows(reset_at);
