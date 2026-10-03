-- name: GetContentConfig :one
SELECT content_config FROM tenants WHERE id=? AND status='active';

-- name: SaveContentConfig :execrows
UPDATE tenants SET content_config=sqlc.arg(config) WHERE id=sqlc.arg(tenant_id) AND status='active' AND content_config=COALESCE(sqlc.arg(previous),X'');

-- name: ListContentConfigs :many
SELECT id,content_config FROM tenants WHERE length(content_config)>0;

-- name: CountContentSecrets :one
SELECT COUNT(*) FROM tenants WHERE length(content_config)>0;
