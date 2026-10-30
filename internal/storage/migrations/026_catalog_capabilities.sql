-- Rebuild with foreign keys disabled by the migration runner to preserve referencing rows.
PRAGMA legacy_alter_table=ON;
CREATE TABLE accounts_next (
    id TEXT PRIMARY KEY NOT NULL,
    provider TEXT NOT NULL DEFAULT 'codex' CHECK (provider IN ('codex', 'claude', 'antigravity', 'xai', 'openai')),
    name TEXT NOT NULL,
    account_id TEXT NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    plan TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    status TEXT NOT NULL CHECK (status IN ('ready', 'unverified', 'reauth_required')),
    credential BLOB NOT NULL,
    expires_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL, max_concurrency INTEGER NOT NULL DEFAULT 30 CHECK(max_concurrency BETWEEN 1 AND 1000), models_snapshot BLOB
    CHECK(models_snapshot IS NULL OR length(models_snapshot) <= 524288), models_revision INTEGER NOT NULL DEFAULT 0,
    tenant_id INTEGER NOT NULL DEFAULT 1 REFERENCES tenants(id),
    proxy_id TEXT REFERENCES proxies(id) ON DELETE RESTRICT,
    reset_config TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(reset_config) AND length(reset_config)<=2048),
    reset_revision INTEGER NOT NULL DEFAULT 0,
    reset_next_run_at INTEGER NOT NULL DEFAULT 0,
    reset_time_zone TEXT NOT NULL DEFAULT '',
    reset_check_at INTEGER NOT NULL DEFAULT 0,
    usage_revision INTEGER NOT NULL DEFAULT 0,
    UNIQUE(tenant_id, provider, account_id)
);
INSERT INTO accounts_next (id,provider,name,account_id,email,plan,enabled,status,credential,expires_at,created_at,updated_at,max_concurrency,models_snapshot,models_revision,tenant_id,proxy_id,reset_config,reset_revision,reset_next_run_at,reset_time_zone,reset_check_at,usage_revision) SELECT id,provider,name,account_id,email,plan,enabled,status,credential,expires_at,created_at,updated_at,max_concurrency,models_snapshot,models_revision,tenant_id,proxy_id,reset_config,reset_revision,reset_next_run_at,reset_time_zone,reset_check_at,usage_revision FROM accounts;
DROP TABLE accounts;
ALTER TABLE accounts_next RENAME TO accounts;
PRAGMA legacy_alter_table=OFF;
CREATE INDEX accounts_enabled ON accounts(enabled, created_at);
CREATE INDEX accounts_tenant ON accounts(tenant_id, id);
CREATE INDEX accounts_proxy_id_idx ON accounts(proxy_id) WHERE proxy_id IS NOT NULL;
CREATE TRIGGER accounts_tenant_update BEFORE UPDATE OF tenant_id ON accounts
WHEN NEW.tenant_id != OLD.tenant_id
BEGIN SELECT RAISE(ABORT, 'tenant_immutable'); END;
CREATE INDEX reset_checks ON accounts(reset_check_at) WHERE provider='codex';
