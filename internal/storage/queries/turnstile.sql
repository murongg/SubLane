-- name: GetTurnstileSetting :one
SELECT value FROM settings WHERE key = 'instance.turnstile';

-- name: SaveTurnstileSetting :exec
INSERT INTO settings(key,value) VALUES('instance.turnstile',sqlc.arg(value))
ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=CURRENT_TIMESTAMP;

-- name: CountTurnstileSettings :one
SELECT count(*) FROM settings WHERE key = 'instance.turnstile';
