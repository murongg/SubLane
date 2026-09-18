CREATE TABLE accounts (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL,
    account_id TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL DEFAULT '',
    plan TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    status TEXT NOT NULL CHECK (status IN ('ready', 'unverified', 'reauth_required')),
    credential BLOB NOT NULL,
    expires_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX accounts_enabled ON accounts(enabled, created_at);

-- Keep deleted-account bindings until expiry so an existing conversation cannot silently switch identity.
CREATE TABLE account_affinity (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_hash BLOB NOT NULL CHECK (length(session_hash) = 32),
    account_id TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, session_hash)
);
CREATE INDEX account_affinity_expiry ON account_affinity(expires_at);
