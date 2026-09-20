-- name: GetCodexVersionState :one
SELECT value FROM settings WHERE key = 'codex.version';

-- name: SaveCodexVersionState :exec
INSERT INTO settings(key,value) VALUES('codex.version',sqlc.arg(value))
ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=CURRENT_TIMESTAMP;
