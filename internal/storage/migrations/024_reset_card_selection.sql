-- Only unresolved attempts retain a provider card ID; legacy attempts keep their original omitted ID.
ALTER TABLE reset_runs ADD COLUMN credit_id TEXT NOT NULL DEFAULT '' CHECK(length(credit_id)<=256);
