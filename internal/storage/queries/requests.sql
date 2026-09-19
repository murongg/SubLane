-- name: RecordRequest :exec
INSERT INTO request_records(user_id,key_id,group_id,account_id,provider,model,transport,operation,started_at,duration_ms,outcome,error_code,upstream_status,input_tokens,output_tokens,cached_tokens)
VALUES(sqlc.arg(user_id),sqlc.arg(key_id),sqlc.arg(group_id),sqlc.arg(account_id),sqlc.arg(provider),sqlc.arg(model),sqlc.arg(transport),sqlc.arg(operation),sqlc.arg(started_at),sqlc.arg(duration_ms),sqlc.arg(outcome),sqlc.arg(error_code),sqlc.narg(upstream_status),sqlc.narg(input_tokens),sqlc.narg(output_tokens),sqlc.narg(cached_tokens));

-- name: PruneRequests :exec
DELETE FROM request_records WHERE request_records.started_at<sqlc.arg(before_time) OR request_records.id<=(SELECT r.id FROM request_records r ORDER BY r.id DESC LIMIT 1 OFFSET 5000);

-- name: ListRequests :many
SELECT r.*,COALESCE(u.username,'') AS username,COALESCE(k.name,'') AS key_name,COALESCE(g.name,'') AS group_name,COALESCE(a.name,'') AS account_name
FROM request_records r LEFT JOIN users u ON u.id=r.user_id LEFT JOIN api_keys k ON k.id=r.key_id LEFT JOIN account_groups g ON g.id=r.group_id LEFT JOIN accounts a ON a.id=r.account_id
WHERE (r.user_id=sqlc.arg(user_id) OR sqlc.arg(user_id)=0) AND (r.id<sqlc.arg(cursor) OR sqlc.arg(cursor)=0) AND (r.account_id=sqlc.arg(account_id) OR sqlc.arg(account_id)='') AND (r.outcome=sqlc.arg(outcome) OR sqlc.arg(outcome)='') AND r.started_at>=sqlc.arg(since)
ORDER BY r.id DESC LIMIT 51;
