CREATE TABLE invitations (
    id INTEGER PRIMARY KEY,
    token_hash BLOB NOT NULL UNIQUE CHECK(length(token_hash) = 32),
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at INTEGER,
    used_by INTEGER REFERENCES users(id),
    CHECK(expires_at > created_at)
);

CREATE INDEX invitations_tenant_expiry ON invitations(tenant_id, expires_at);
