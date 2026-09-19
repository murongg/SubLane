-- name: GetMemberLimits :one
SELECT u.id,u.role,u.enabled,CAST(COALESCE(l.requests_per_minute,0) AS INTEGER) AS requests_per_minute,CAST(COALESCE(l.max_concurrency,0) AS INTEGER) AS max_concurrency
FROM users u LEFT JOIN member_limits l ON l.user_id=u.id WHERE u.id=sqlc.arg(user_id);

-- name: SetMemberLimits :execrows
INSERT INTO member_limits(user_id,requests_per_minute,max_concurrency)
SELECT id,sqlc.arg(requests_per_minute),sqlc.arg(max_concurrency) FROM users WHERE id=sqlc.arg(user_id) AND role='member'
ON CONFLICT(user_id) DO UPDATE SET requests_per_minute=excluded.requests_per_minute,max_concurrency=excluded.max_concurrency;

-- name: GetMemberWindow :one
SELECT window_start,requests FROM member_rate WHERE user_id=sqlc.arg(user_id);

-- name: TakeMemberRate :execrows
INSERT INTO member_rate(user_id,window_start,requests)
SELECT sqlc.arg(user_id),sqlc.arg(window_start),1
WHERE NOT EXISTS(SELECT 1 FROM member_rate r WHERE r.user_id=sqlc.arg(user_id) AND r.window_start=sqlc.arg(window_start) AND r.requests>=sqlc.arg(rate_limit))
ON CONFLICT(user_id) DO UPDATE SET window_start=excluded.window_start,requests=CASE WHEN member_rate.window_start=excluded.window_start THEN member_rate.requests+1 ELSE 1 END;
