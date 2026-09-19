CREATE TABLE account_groups (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE COLLATE NOCASE,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    CHECK (id != 1 OR (name = 'Default' AND enabled = 1))
);
INSERT INTO account_groups(id,name,created_at,updated_at) VALUES(1,'Default',unixepoch(),unixepoch());
CREATE TABLE group_accounts (
    group_id INTEGER NOT NULL REFERENCES account_groups(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    PRIMARY KEY(group_id,account_id)
);
CREATE INDEX group_accounts_account ON group_accounts(account_id);
CREATE TABLE group_members (
    group_id INTEGER NOT NULL REFERENCES account_groups(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY(group_id,user_id)
);
CREATE INDEX group_members_user ON group_members(user_id);
INSERT INTO group_accounts SELECT 1,id FROM accounts;
INSERT INTO group_members SELECT 1,id FROM users;

CREATE TABLE api_keys_next (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id INTEGER NOT NULL DEFAULT 1 REFERENCES account_groups(id),
    name TEXT NOT NULL,
    prefix TEXT NOT NULL,
    token_hash BLOB NOT NULL UNIQUE CHECK(length(token_hash)=32),
    created_at INTEGER NOT NULL,
    last_used_at INTEGER,
    revoked_at INTEGER
);
INSERT INTO api_keys_next SELECT id,user_id,1,name,prefix,token_hash,created_at,last_used_at,revoked_at FROM api_keys;
DROP TABLE api_keys;
ALTER TABLE api_keys_next RENAME TO api_keys;
CREATE INDEX api_keys_owner ON api_keys(user_id,id);
CREATE INDEX api_keys_active_owner ON api_keys(user_id) WHERE revoked_at IS NULL;
CREATE INDEX api_keys_group ON api_keys(group_id);

-- Keep legacy session hashes and bindings unchanged inside the default pool.
CREATE TABLE account_affinity_next (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id INTEGER NOT NULL DEFAULT 1 REFERENCES account_groups(id) ON DELETE CASCADE,
    session_hash BLOB NOT NULL CHECK(length(session_hash)=32),
    provider TEXT NOT NULL DEFAULT 'codex',
    account_id TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    PRIMARY KEY(user_id,group_id,session_hash,provider)
);
INSERT INTO account_affinity_next SELECT user_id,1,session_hash,provider,account_id,expires_at FROM account_affinity;
DROP TABLE account_affinity;
ALTER TABLE account_affinity_next RENAME TO account_affinity;
CREATE INDEX account_affinity_expiry ON account_affinity(expires_at);
