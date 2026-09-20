ALTER TABLE request_records ADD COLUMN request_id TEXT NOT NULL DEFAULT '' CHECK(length(request_id) <= 64);
ALTER TABLE request_records ADD COLUMN first_token_ms INTEGER CHECK(first_token_ms >= 0);
CREATE INDEX request_records_request_id ON request_records(request_id) WHERE request_id != '';

-- Bind observations to the account lifecycle without discarding existing quota displays.
ALTER TABLE account_usage ADD COLUMN revision INTEGER NOT NULL DEFAULT 0;
UPDATE account_usage SET revision = (SELECT models_revision FROM accounts WHERE accounts.id = account_usage.account_id);
