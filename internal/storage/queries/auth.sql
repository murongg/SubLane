-- name: HasAdministrator :one
SELECT EXISTS(SELECT 1 FROM users WHERE id = 1 AND role = 'admin');

-- name: GetSessionUser :one
SELECT u.id, u.username, u.role
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = sqlc.arg(token_hash) AND s.expires_at > sqlc.arg(now) AND u.enabled = 1;

-- name: CreateAdministrator :execrows
INSERT INTO users(id, username, role, password_hash, created_at)
SELECT 1, sqlc.arg(username), 'admin', sqlc.arg(password_hash), sqlc.arg(created_at)
WHERE NOT EXISTS(SELECT 1 FROM users WHERE id = 1);

-- name: GetLoginUser :one
SELECT id, username, role, password_hash FROM users
WHERE username = sqlc.arg(username) AND enabled = 1;

-- name: GetEnabledUser :one
SELECT id, username, role FROM users WHERE id = sqlc.arg(id) AND enabled = 1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= sqlc.arg(now);

-- name: CreateSession :exec
INSERT INTO sessions(token_hash, user_id, created_at, expires_at)
VALUES (sqlc.arg(token_hash), sqlc.arg(user_id), sqlc.arg(created_at), sqlc.arg(expires_at));

-- name: TrimSessions :exec
DELETE FROM sessions WHERE token_hash IN (
    SELECT s.token_hash FROM sessions s
    WHERE s.user_id = sqlc.arg(user_id) AND s.token_hash != sqlc.arg(current_token_hash)
    ORDER BY s.created_at DESC, s.rowid DESC LIMIT -1 OFFSET 4
);

-- name: RevokeSession :exec
DELETE FROM sessions WHERE token_hash = sqlc.arg(token_hash);
