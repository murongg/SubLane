-- Retain complete per-model quota snapshots, including hidden models.
CREATE TABLE account_usage_next (
    account_id TEXT PRIMARY KEY NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    snapshot BLOB NOT NULL CHECK (length(snapshot) <= 262144),
    updated_at INTEGER NOT NULL CHECK (updated_at > 0),
    revision INTEGER NOT NULL DEFAULT 0
);
INSERT INTO account_usage_next (account_id, snapshot, updated_at, revision)
SELECT account_id, snapshot, updated_at, revision FROM account_usage;
DROP TABLE account_usage;
ALTER TABLE account_usage_next RENAME TO account_usage;
