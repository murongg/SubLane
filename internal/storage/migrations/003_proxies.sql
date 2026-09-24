CREATE TABLE proxies (
    id TEXT PRIMARY KEY NOT NULL,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    address BLOB NOT NULL CHECK(length(address) <= 8192),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    revision INTEGER NOT NULL DEFAULT 0,
    checked_at INTEGER NOT NULL DEFAULT 0,
    reachable INTEGER NOT NULL DEFAULT 0 CHECK(reachable IN (0,1)),
    exit_ip TEXT NOT NULL DEFAULT '' CHECK(length(exit_ip) <= 45),
    country TEXT NOT NULL DEFAULT '' CHECK(length(country) <= 2),
    region TEXT NOT NULL DEFAULT '' CHECK(length(region) <= 128),
    city TEXT NOT NULL DEFAULT '' CHECK(length(city) <= 128),
    latency_ms INTEGER NOT NULL DEFAULT 0 CHECK(latency_ms BETWEEN 0 AND 30000),
    check_error TEXT NOT NULL DEFAULT '' CHECK(length(check_error) <= 32),
    UNIQUE(tenant_id, name)
);

ALTER TABLE accounts ADD COLUMN proxy_id TEXT REFERENCES proxies(id) ON DELETE RESTRICT;
CREATE INDEX accounts_proxy_id_idx ON accounts(proxy_id) WHERE proxy_id IS NOT NULL;
