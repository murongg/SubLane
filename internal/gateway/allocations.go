package gateway

import (
	"context"
	"database/sql"
	"errors"

	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/storage/db"
)

func (s *Service) Allocations() *allocations.Service { return allocations.New(s.db) }
func (s *Service) RefreshAllocation(ctx context.Context, id int64) error {
	row, err := s.queries.GetAllocationScheme(ctx, id)
	if err != nil {
		return allocations.ErrNotFound
	}
	ids, err := s.queries.ListGroupAccounts(ctx, row.GroupID)
	if err != nil {
		return err
	}
	for _, account := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Persist successful peers even when another subscription is unavailable.
		// Admission independently validates each account's persisted snapshot.
		_, _ = s.RefreshUsage(ctx, account)
	}
	return s.Allocations().Refresh(ctx, id)
}
func (s *Service) refreshAllocation(ctx context.Context, e *observation) error {
	id, err := allocations.KeyScheme(ctx, s.queries, e.record.KeyID, e.record.UserID, e.record.GroupID, s.now().Unix())
	if err != nil || id == 0 {
		return err
	}
	rev, err := allocations.Current(ctx, s.queries, id, s.now().Unix())
	if err != nil {
		return err
	}
	if rev.Config.Mode != "ratio" {
		return nil
	}
	// The metadata request runs outside both admission lock and database transaction.
	return s.RefreshAllocation(ctx, id)
}
func (e *observation) settleAllocation(ctx context.Context, q *db.Queries) error {
	if !e.allocationTracked {
		return nil
	}
	value := func(v *int64) int64 {
		if v != nil {
			return *v
		}
		return 0
	}
	known := e.record.InputTokens != nil && e.record.OutputTokens != nil && (e.record.Outcome == "success" || e.record.Outcome == "incomplete")
	rejected := e.record.UpstreamStatus != nil && *e.record.UpstreamStatus >= 400
	err := allocations.Finish(ctx, q, e.record.RequestID, allocations.Completion{Input: value(e.record.InputTokens), Output: value(e.record.OutputTokens), Cached: value(e.record.CachedTokens), Known: known, Dispatched: e.budgetDispatched, Rejected: rejected}, e.service.now().UnixMilli(), false)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTokenAccounting
	}
	return err
}

func (s *Service) PrepareAllocations(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recoverBudgets(ctx)
}
