CREATE TABLE account_usage (
    account_id TEXT PRIMARY KEY NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    snapshot BLOB NOT NULL CHECK (length(snapshot) <= 131072),
    updated_at INTEGER NOT NULL CHECK (updated_at > 0)
);
