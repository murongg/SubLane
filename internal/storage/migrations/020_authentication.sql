ALTER TABLE users ADD COLUMN totp_secret BLOB CHECK(totp_secret IS NULL OR length(totp_secret) <= 512);
ALTER TABLE users ADD COLUMN totp_step INTEGER NOT NULL DEFAULT -1;
ALTER TABLE users ADD COLUMN security_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN passkey_handle BLOB CHECK(passkey_handle IS NULL OR length(passkey_handle) = 32);
CREATE UNIQUE INDEX users_passkey_handle ON users(passkey_handle) WHERE passkey_handle IS NOT NULL;

CREATE TABLE recovery_codes (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash BLOB NOT NULL CHECK(length(code_hash) = 32),
    PRIMARY KEY(user_id, code_hash)
);
CREATE TABLE passkeys (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id BLOB NOT NULL UNIQUE CHECK(length(credential_id) BETWEEN 1 AND 1024),
    credential TEXT NOT NULL CHECK(length(credential) <= 32768),
    name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 64),
    created_at INTEGER NOT NULL,
    last_used_at INTEGER
);
CREATE INDEX passkeys_user ON passkeys(user_id);
CREATE TABLE auth_challenges (
    token_hash BLOB PRIMARY KEY NOT NULL CHECK(length(token_hash) = 32),
    kind TEXT NOT NULL CHECK(kind IN ('totp', 'register', 'login')),
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    session_hash BLOB NOT NULL,
    security_version INTEGER NOT NULL,
    data TEXT NOT NULL CHECK(length(data) <= 65536),
    expires_at INTEGER NOT NULL
);
CREATE INDEX auth_challenges_expiry ON auth_challenges(expires_at);
