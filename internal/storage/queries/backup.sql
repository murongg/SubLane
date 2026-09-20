-- name: RevokeRestoredSessions :exec
DELETE FROM sessions;

-- name: CountOversizedBackupRecords :one
-- Match the account owner's bounded verification loop before scanning any credential rows.
SELECT count(*) FROM (
    SELECT 1 WHERE (SELECT count(*) FROM accounts) > 100
    UNION ALL
    SELECT 1 FROM accounts WHERE octet_length(id) > 64 OR octet_length(provider) > 32
      OR octet_length(name) > 256 OR octet_length(account_id) > 1024
      OR octet_length(email) > 1280 OR octet_length(plan) > 256
      OR octet_length(credential) > 262144
    UNION ALL
    SELECT 1 FROM api_keys WHERE octet_length(encrypted_secret) > 512 OR octet_length(token_hash) > 32
    LIMIT 1
);
