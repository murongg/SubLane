ALTER TABLE api_keys ADD COLUMN encrypted_secret BLOB
    CHECK(encrypted_secret IS NULL OR length(encrypted_secret) BETWEEN 1 AND 512);
-- Preserve ciphertext byte-for-byte: encryption binds it to the existing owner and key IDs.
UPDATE api_keys SET encrypted_secret=(SELECT secret FROM api_key_secrets WHERE key_id=api_keys.id)
WHERE id IN (SELECT key_id FROM api_key_secrets);

ALTER TABLE users ADD COLUMN requests_per_minute INTEGER NOT NULL DEFAULT 0
    CHECK(requests_per_minute BETWEEN 0 AND 6000);
ALTER TABLE users ADD COLUMN max_concurrency INTEGER NOT NULL DEFAULT 0
    CHECK(max_concurrency BETWEEN 0 AND 8);
UPDATE users SET
    requests_per_minute=(SELECT requests_per_minute FROM member_limits WHERE user_id=users.id),
    max_concurrency=(SELECT max_concurrency FROM member_limits WHERE user_id=users.id)
WHERE id IN (SELECT user_id FROM member_limits);

-- These timestamps describe different collection windows. Neither may be reset to upgrade time.
INSERT INTO settings(key,value)
SELECT 'usage.daily.started_at',CAST(started_at AS TEXT) FROM usage_coverage WHERE id=1
ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=CURRENT_TIMESTAMP;
INSERT INTO settings(key,value)
SELECT 'usage.hourly.started_at',CAST(started_at AS TEXT) FROM usage_hourly_coverage WHERE id=1
ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=CURRENT_TIMESTAMP;

-- The migration runner commits copies and removals together.
DROP TABLE api_key_secrets;
DROP TABLE member_limits;
DROP TABLE usage_coverage;
DROP TABLE usage_hourly_coverage;
