ALTER TABLE accounts ADD COLUMN models_snapshot BLOB
    CHECK(models_snapshot IS NULL OR length(models_snapshot) <= 131072);
-- Explicit account lifecycle changes invalidate any in-flight discovery, even if the token is unchanged.
ALTER TABLE accounts ADD COLUMN models_revision INTEGER NOT NULL DEFAULT 0;
