-- name: CreateAuditEvent :exec
INSERT INTO audit_events(actor_id,actor_name,actor_role,source,action,resource,resource_id,outcome,http_status,created_at)
VALUES(sqlc.arg(actor_id),sqlc.arg(actor_name),sqlc.arg(actor_role),sqlc.arg(source),sqlc.arg(action),sqlc.arg(resource),sqlc.arg(resource_id),sqlc.arg(outcome),sqlc.narg(http_status),sqlc.arg(created_at));

-- name: PruneAuditEvents :exec
DELETE FROM audit_events WHERE audit_events.created_at<sqlc.arg(oldest) OR audit_events.id IN (SELECT id FROM audit_events ORDER BY id DESC LIMIT -1 OFFSET 10000);

-- name: ListAuditEvents :many
SELECT * FROM audit_events
WHERE (id<sqlc.arg(before_id) OR sqlc.arg(before_id)=0)
AND (resource=sqlc.arg(resource) OR sqlc.arg(resource)='')
AND (outcome=sqlc.arg(outcome) OR sqlc.arg(outcome)='')
ORDER BY id DESC LIMIT 51;
