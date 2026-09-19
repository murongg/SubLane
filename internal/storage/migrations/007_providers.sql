CREATE TABLE accounts_next (
    id TEXT PRIMARY KEY NOT NULL,
    provider TEXT NOT NULL DEFAULT 'codex' CHECK (provider IN ('codex', 'claude', 'antigravity')),
    name TEXT NOT NULL,
    account_id TEXT NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    plan TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    status TEXT NOT NULL CHECK (status IN ('ready', 'unverified', 'reauth_required')),
    credential BLOB NOT NULL,
    expires_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(provider, account_id)
);
INSERT INTO accounts_next SELECT id, 'codex', name, account_id, email, plan, enabled, status, credential, expires_at, created_at, updated_at FROM accounts;
-- Preserve dependent snapshots before replacing their parent; DROP would otherwise cascade them away.
CREATE TABLE account_usage_next (
    account_id TEXT PRIMARY KEY NOT NULL REFERENCES accounts_next(id) ON DELETE CASCADE,
    snapshot BLOB NOT NULL CHECK (length(snapshot) <= 131072),
    updated_at INTEGER NOT NULL CHECK (updated_at > 0)
);
INSERT INTO account_usage_next SELECT * FROM account_usage;
DROP TABLE account_usage;
DROP TABLE accounts;
ALTER TABLE accounts_next RENAME TO accounts;
ALTER TABLE account_usage_next RENAME TO account_usage;
CREATE INDEX accounts_enabled ON accounts(enabled, created_at);
CREATE TABLE account_affinity_next (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_hash BLOB NOT NULL CHECK (length(session_hash) = 32),
    provider TEXT NOT NULL DEFAULT 'codex',
    account_id TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, session_hash, provider)
);
INSERT INTO account_affinity_next SELECT user_id, session_hash, 'codex', account_id, expires_at FROM account_affinity;
DROP TABLE account_affinity;
ALTER TABLE account_affinity_next RENAME TO account_affinity;
CREATE INDEX account_affinity_expiry ON account_affinity(expires_at);
