ALTER TABLE account_groups ADD COLUMN restricted_models INTEGER NOT NULL DEFAULT 0 CHECK(restricted_models IN (0,1));
CREATE TABLE group_models (
    group_id INTEGER NOT NULL REFERENCES account_groups(id) ON DELETE CASCADE,
    model TEXT NOT NULL,
    PRIMARY KEY(group_id,model)
);
