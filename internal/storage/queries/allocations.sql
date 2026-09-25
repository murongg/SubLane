-- name: ListAllocationSchemes :many
SELECT s.*,g.name AS group_name FROM allocation_schemes s
JOIN account_groups g ON g.id=s.group_id WHERE g.tenant_id=sqlc.arg(tenant_id) ORDER BY s.id;
-- name: GetAllocationScheme :one
SELECT s.*,g.name AS group_name FROM allocation_schemes s
JOIN account_groups g ON g.id=s.group_id WHERE s.id=?;
-- name: GetTenantAllocationScheme :one
SELECT s.*,g.name AS group_name FROM allocation_schemes s
JOIN account_groups g ON g.id=s.group_id WHERE s.id=sqlc.arg(id) AND g.tenant_id=sqlc.arg(tenant_id);
-- name: GetPoolAllocation :one
SELECT id FROM allocation_schemes WHERE group_id=?;
-- name: CreateAllocationScheme :execlastid
INSERT INTO allocation_schemes(name,group_id,enabled,created_at) VALUES(?,?,?,?);
-- name: UpdateAllocationScheme :exec
UPDATE allocation_schemes SET name=?,enabled=? WHERE id=?;
-- name: CurrentAllocationRevision :one
SELECT * FROM allocation_revisions WHERE scheme_id=? AND effective_at<=? ORDER BY effective_at DESC LIMIT 1;
-- name: NextAllocationRevision :one
SELECT * FROM allocation_revisions WHERE scheme_id=? AND effective_at>? ORDER BY effective_at LIMIT 1;
-- name: GetAllocationRevision :one
SELECT * FROM allocation_revisions WHERE id=?;
-- name: SaveAllocationRevision :execlastid
INSERT INTO allocation_revisions(scheme_id,effective_at,config) VALUES(?,?,?);
-- name: DeleteNextAllocationRevisions :exec
DELETE FROM allocation_revisions WHERE scheme_id=? AND effective_at>?;
-- name: AllocationPoolAccounts :many
SELECT a.id,a.provider,a.models_revision,(SELECT count(*) FROM group_accounts other WHERE other.account_id=a.id AND other.group_id<>ga.group_id) AS shared
FROM group_accounts ga JOIN accounts a ON a.id=ga.account_id WHERE ga.group_id=? ORDER BY a.id;
-- name: CanUseAllocation :one
SELECT EXISTS(SELECT 1 FROM allocation_schemes s
JOIN effective_group_access access ON access.group_id=s.group_id
WHERE s.id=? AND access.user_id=? AND s.enabled=1);
-- name: BindKeyAllocation :exec
INSERT INTO allocation_keys(key_id,scheme_id) VALUES(?,?);
-- name: GetKeyAllocation :one
SELECT scheme_id FROM allocation_keys WHERE key_id=?;
-- name: BeginAllocationEntry :exec
INSERT INTO allocation_entries(request_id,scheme_id,revision_id,user_id,account_id,model,mode,window_start,reset_at,started_at,state)
VALUES(?,?,?,?,?,?,?,?,?,?,'active');
-- name: GetAllocationEntry :one
SELECT * FROM allocation_entries WHERE request_id=?;
-- name: FinishAllocationEntry :exec
UPDATE allocation_entries SET finished_at=?,state=?,input_tokens=?,output_tokens=?,cached_tokens=?,cost=?,manual=? WHERE request_id=?;
-- name: RecoverAllocationEntries :exec
UPDATE allocation_entries SET state='pending' WHERE state='active' AND EXISTS(
SELECT 1 FROM allocation_schemes s JOIN account_groups g ON g.id=s.group_id
WHERE s.id=allocation_entries.scheme_id AND g.tenant_id=sqlc.arg(tenant_id));
-- name: AllocationMemberUsage :one
SELECT CAST(COALESCE(sum(cost),0) AS INTEGER) AS used,CAST(COALESCE(sum(input_tokens+output_tokens),0) AS INTEGER) AS tokens
FROM allocation_entries WHERE scheme_id=? AND user_id=? AND window_start=? AND mode=?;
-- name: AllocationMemberPending :one
SELECT count(*) FROM allocation_entries WHERE scheme_id=? AND user_id=? AND state='pending';
-- name: ListAllocationPending :many
SELECT * FROM allocation_entries WHERE scheme_id=sqlc.arg(scheme_id) AND (sqlc.arg(user_id)=0 OR user_id=sqlc.arg(user_id)) AND (state='pending' OR state='observed') ORDER BY started_at LIMIT 256;
-- name: ListAllocationWindows :many
SELECT * FROM allocation_windows WHERE scheme_id=? ORDER BY account_id,kind,reset_at DESC;
-- name: CurrentAccountAllocationWindows :many
SELECT * FROM allocation_windows WHERE scheme_id=? AND account_id=? AND reset_at>? ORDER BY kind;
-- name: FindAllocationWindow :one
SELECT * FROM allocation_windows WHERE scheme_id=? AND account_id=? AND kind=? AND reset_at=?;
-- name: CreateAllocationWindow :execlastid
INSERT INTO allocation_windows(scheme_id,account_id,kind,reset_at,account_revision,observed_at,observed_points,baseline_points)
VALUES(?,?,?,?,?,?,?,?);
-- name: ObserveAllocationWindow :exec
UPDATE allocation_windows SET observed_at=?,observed_points=?,unassigned=unassigned+? WHERE id=?;
-- name: AddAllocationWindowMember :exec
INSERT INTO allocation_window_members(window_id,user_id,allowance) VALUES(?,?,?);
-- name: AllocationWindowUsage :many
SELECT m.user_id,m.allowance,CAST(COALESCE(sum(d.points),0) AS INTEGER) AS used
FROM allocation_window_members m LEFT JOIN allocation_entries e ON e.user_id=m.user_id
LEFT JOIN allocation_debits d ON d.request_id=e.request_id AND d.window_id=m.window_id
WHERE m.window_id=sqlc.arg(window_id) AND (sqlc.arg(user_id)=0 OR m.user_id=sqlc.arg(user_id)) GROUP BY m.user_id,m.allowance ORDER BY m.user_id;
-- name: BeginAllocationDebit :exec
INSERT INTO allocation_debits(request_id,window_id) VALUES(?,?);
-- name: ListAllocationDebits :many
SELECT d.*,e.cost,e.state,e.finished_at,e.started_at,e.user_id FROM allocation_debits d
JOIN allocation_entries e ON e.request_id=d.request_id WHERE window_id=? AND reconciled=0 ORDER BY e.started_at,e.request_id;
-- name: SetAllocationDebit :exec
UPDATE allocation_debits SET points=?,reconciled=1 WHERE request_id=? AND window_id=?;
-- name: SettleReconciledAllocationEntries :exec
UPDATE allocation_entries SET state='settled' WHERE scheme_id=? AND state='observed'
AND NOT EXISTS(SELECT 1 FROM allocation_debits d WHERE d.request_id=allocation_entries.request_id AND d.reconciled=0);
-- name: AllocationAccountUnfinished :one
SELECT count(*) FROM allocation_entries WHERE account_id=? AND state IN ('active','pending');
-- name: GetRequestAllocationDebits :many
SELECT d.*,w.kind,w.reset_at,w.account_id FROM allocation_debits d JOIN allocation_windows w ON w.id=d.window_id WHERE request_id=?;
-- name: AllocationUnassigned :one
SELECT CAST(COALESCE(sum(unassigned),0) AS INTEGER) FROM allocation_windows WHERE scheme_id=?;
-- name: ClearAllocationUnassigned :exec
UPDATE allocation_windows SET unassigned=0 WHERE scheme_id=?;
-- name: MarkExpiredAllocationEntries :exec
UPDATE allocation_entries SET state='pending' WHERE allocation_entries.scheme_id=sqlc.arg(scheme_id) AND state='observed'
AND EXISTS(SELECT 1 FROM allocation_debits d JOIN allocation_windows w ON w.id=d.window_id
WHERE d.request_id=allocation_entries.request_id AND d.reconciled=0 AND w.reset_at<=sqlc.arg(reset_at));
-- name: AllocationAccountObserved :one
SELECT count(*) FROM allocation_entries WHERE scheme_id=? AND account_id=? AND state IN ('active','observed');
-- name: AllocationAccountAwaiting :one
SELECT count(*) AS count, CAST(COALESCE(min(finished_at),0) AS INTEGER) AS oldest
FROM allocation_entries WHERE scheme_id=? AND account_id=? AND state='observed';
-- name: PruneAllocations :exec
DELETE FROM allocation_entries WHERE state='settled' AND (
 (mode<>'ratio' AND allocation_entries.reset_at<sqlc.arg(before)) OR
 (mode='ratio' AND finished_at<sqlc.arg(before)*1000 AND NOT EXISTS(
 SELECT 1 FROM allocation_debits d JOIN allocation_windows w ON w.id=d.window_id
 WHERE d.request_id=allocation_entries.request_id AND (d.reconciled=0 OR w.reset_at>=sqlc.arg(before))))
);
-- name: PruneAllocationWindows :exec
DELETE FROM allocation_windows WHERE reset_at<? AND unassigned=0
AND NOT EXISTS(SELECT 1 FROM allocation_debits WHERE window_id=allocation_windows.id);
-- name: AdvanceManualAllocationWindow :exec
UPDATE allocation_windows SET observed_points=observed_points+? WHERE id=?;
-- name: GetAllocationWindow :one
SELECT * FROM allocation_windows WHERE id=?;
-- name: AllocationWindowTokens :one
SELECT CAST(COALESCE(sum(e.input_tokens+e.output_tokens),0) AS INTEGER) FROM allocation_debits d
JOIN allocation_entries e ON e.request_id=d.request_id WHERE d.window_id=? AND e.user_id=?;
-- name: AllocationMemberUnresolved :one
SELECT count(*) FROM allocation_entries e WHERE e.scheme_id=? AND e.user_id=? AND (e.state='pending'
OR (e.state='observed' AND EXISTS(SELECT 1 FROM allocation_debits d JOIN allocation_windows w ON w.id=d.window_id
WHERE d.request_id=e.request_id AND d.reconciled=0 AND w.reset_at<=?)));

-- name: AccountHasAllocation :one
SELECT EXISTS(SELECT 1 FROM group_accounts ga JOIN allocation_schemes s ON s.group_id=ga.group_id WHERE ga.account_id=?);

-- name: RevalidateAllocationWindow :exec
UPDATE allocation_windows SET account_revision=? WHERE id=?;
