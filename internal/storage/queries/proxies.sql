-- name: CountProxies :one
SELECT count(*) FROM proxies WHERE tenant_id = sqlc.arg(tenant_id);

-- name: ListProxies :many
SELECT p.id, p.name, p.address, p.created_at, p.updated_at, p.checked_at, p.reachable,
 p.exit_ip, p.country, p.region, p.city, p.latency_ms, p.check_error,
 (SELECT count(*) FROM accounts a WHERE a.proxy_id = p.id) AS account_count
FROM proxies p WHERE p.tenant_id = sqlc.arg(tenant_id)
ORDER BY p.created_at DESC, p.id DESC LIMIT 32;

-- name: GetProxy :one
SELECT * FROM proxies WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: CreateProxy :execrows
INSERT INTO proxies(id, tenant_id, name, address, created_at, updated_at)
VALUES(sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(name), sqlc.arg(address), sqlc.arg(created_at), sqlc.arg(updated_at))
ON CONFLICT(tenant_id, name) DO NOTHING;

-- name: UpdateProxy :execrows
UPDATE proxies SET name = sqlc.arg(name),
 revision = revision + CASE WHEN address != sqlc.arg(address) THEN 1 ELSE 0 END,
 checked_at = CASE WHEN address != sqlc.arg(address) THEN 0 ELSE checked_at END,
 reachable = CASE WHEN address != sqlc.arg(address) THEN 0 ELSE reachable END,
 exit_ip = CASE WHEN address != sqlc.arg(address) THEN '' ELSE exit_ip END,
 country = CASE WHEN address != sqlc.arg(address) THEN '' ELSE country END,
 region = CASE WHEN address != sqlc.arg(address) THEN '' ELSE region END,
 city = CASE WHEN address != sqlc.arg(address) THEN '' ELSE city END,
 latency_ms = CASE WHEN address != sqlc.arg(address) THEN 0 ELSE latency_ms END,
 check_error = CASE WHEN address != sqlc.arg(address) THEN '' ELSE check_error END,
 address = sqlc.arg(address), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: SaveProxyCheck :execrows
UPDATE proxies SET checked_at=sqlc.arg(checked_at), reachable=sqlc.arg(reachable),
 exit_ip=sqlc.arg(exit_ip), country=sqlc.arg(country), region=sqlc.arg(region),
 city=sqlc.arg(city), latency_ms=sqlc.arg(latency_ms), check_error=sqlc.arg(check_error)
WHERE id=sqlc.arg(id) AND tenant_id=sqlc.arg(tenant_id) AND revision=sqlc.arg(revision);

-- name: DeleteProxy :execrows
DELETE FROM proxies WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: DeleteFailedProxies :execrows
DELETE FROM proxies WHERE proxies.tenant_id=sqlc.arg(tenant_id)
AND proxies.checked_at >= sqlc.arg(checked_since) AND proxies.reachable=0 AND proxies.check_error='connection_failed'
AND NOT EXISTS(SELECT 1 FROM accounts WHERE accounts.proxy_id=proxies.id);

-- name: CountProxyAccounts :one
SELECT count(*) FROM accounts WHERE proxy_id = sqlc.arg(proxy_id);

-- name: InvalidateProxyCatalogs :exec
UPDATE accounts SET models_snapshot = NULL, models_revision = models_revision + 1
WHERE proxy_id = sqlc.arg(proxy_id);

-- name: BindAccountProxy :execrows
UPDATE accounts SET proxy_id = sqlc.narg(proxy_id), updated_at = sqlc.arg(updated_at),
 models_snapshot = NULL, models_revision = models_revision + 1
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);
