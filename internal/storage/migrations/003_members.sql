CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    username TEXT NOT NULL UNIQUE COLLATE NOCASE,
    role TEXT NOT NULL CHECK (role IN ('admin', 'member')),
    password_hash TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at INTEGER NOT NULL,
    CHECK ((id = 1 AND role = 'admin' AND enabled = 1) OR (id > 1 AND role = 'member'))
);

INSERT INTO users(id, username, role, password_hash, enabled, created_at)
SELECT id, username, 'admin', password_hash, 1, created_at FROM administrators;

CREATE TABLE sessions (
    token_hash BLOB PRIMARY KEY NOT NULL CHECK (length(token_hash) = 32),
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

INSERT INTO sessions(token_hash, user_id, created_at, expires_at)
SELECT token_hash, administrator_id, created_at, expires_at FROM admin_sessions;

CREATE INDEX sessions_expiry ON sessions(expires_at);
CREATE INDEX sessions_user ON sessions(user_id, created_at);

DROP TABLE admin_sessions;
DROP TABLE administrators;
