-- Legacy hashes stay valid; only newly created keys have a recoverable encrypted value.
CREATE TABLE api_key_secrets (
    key_id INTEGER PRIMARY KEY REFERENCES api_keys(id) ON DELETE CASCADE,
    secret BLOB NOT NULL CHECK(length(secret) BETWEEN 1 AND 512)
);
