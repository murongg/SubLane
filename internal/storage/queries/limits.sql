-- name: GetMemberLimits :one
SELECT id,role,enabled,requests_per_minute,max_concurrency FROM users WHERE id=sqlc.arg(user_id);

-- name: SetMemberLimits :execrows
UPDATE users SET requests_per_minute=sqlc.arg(requests_per_minute),max_concurrency=sqlc.arg(max_concurrency)
WHERE id=sqlc.arg(user_id) AND role='member';

-- name: GetMemberWindow :one
SELECT window_start,requests FROM member_rate WHERE user_id=sqlc.arg(user_id);

-- name: TakeMemberRate :execrows
INSERT INTO member_rate(user_id,window_start,requests)
SELECT sqlc.arg(user_id),sqlc.arg(window_start),1
WHERE NOT EXISTS(SELECT 1 FROM member_rate r WHERE r.user_id=sqlc.arg(user_id) AND r.window_start=sqlc.arg(window_start) AND r.requests>=sqlc.arg(rate_limit))
ON CONFLICT(user_id) DO UPDATE SET window_start=excluded.window_start,requests=CASE WHEN member_rate.window_start=excluded.window_start THEN member_rate.requests+1 ELSE 1 END;
