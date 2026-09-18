-- name: ListMembers :many
SELECT id, username, role, enabled, created_at FROM users
WHERE role = 'member' AND (id < sqlc.arg(before_id) OR sqlc.arg(before_id) = 0)
ORDER BY id DESC LIMIT 51;

-- name: CreateMember :one
INSERT INTO users(username, role, password_hash, created_at)
VALUES (sqlc.arg(username), 'member', sqlc.arg(password_hash), sqlc.arg(created_at))
ON CONFLICT(username) DO NOTHING
RETURNING id, username, role, enabled, created_at;

-- name: GetMember :one
SELECT id, username, role, enabled, created_at FROM users
WHERE id = sqlc.arg(id) AND role = 'member';

-- name: SetMemberEnabled :exec
UPDATE users SET enabled = sqlc.arg(enabled) WHERE id = sqlc.arg(id);

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = sqlc.arg(user_id);
