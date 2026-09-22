package gateway

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/pricing"
	"github.com/murongg/SubLane/internal/storage/db"
)

func (s *Service) Allocations() *allocations.Service {
	return allocations.NewWithPricing(s.db, s.pricing)
}

func (s *Service) Pricing() *pricing.Service { return s.pricing }
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
	row, err := s.queries.GetAllocationScheme(ctx, id)
	if err != nil {
		return err
	}
	ids, err := s.queries.ListGroupAccounts(ctx, row.GroupID)
	if err != nil {
		return err
	}
	for _, account := range ids {
		waiting, err := s.queries.AllocationAccountAwaiting(ctx, db.AllocationAccountAwaitingParams{SchemeID: id, AccountID: account})
		if err != nil {
			return err
		}
		// Fresh response observations suffice for routing. Outstanding debits need
		// a poll that actually starts after completion, so never skip that poll.
		if waiting.Count > 0 {
			_, _ = s.RefreshUsage(ctx, account)
		} else {
			_, _ = s.Usage(ctx, account)
		}
	}
	return s.Allocations().Refresh(ctx, id)
}

// Called under s.mu after settlement commits. One bounded worker per account
// follows the last request too; browser polling and further traffic are optional.
func (s *Service) scheduleAllocationSync(scheme int64, account string) {
	if s.closed {
		return
	}
	if s.allocationSync == nil {
		s.allocationSync = map[string]uint64{}
	}
	if s.allocationSync[account] > 0 {
		s.allocationSync[account]++
		return
	}
	s.allocationSync[account] = 1
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		defer func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			// Completion and worker removal share the admission lock. A request
			// finishing during the final check must not lose its automatic retry.
			again := s.allocationSync[account] > 1
			delete(s.allocationSync, account)
			if again {
				s.scheduleAllocationSync(scheme, account)
			}
		}()
		ctx, cancel := context.WithTimeout(s.runContext, 2*time.Minute)
		defer cancel()
		for _, seconds := range []int{5, 10, 20, 30, 30, 20} {
			waiting, err := s.queries.AllocationAccountAwaiting(ctx, db.AllocationAccountAwaitingParams{SchemeID: scheme, AccountID: account})
			if err != nil || waiting.Count == 0 {
				return
			}
			timer := time.NewTimer(time.Duration(seconds) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if _, err := s.RefreshUsage(ctx, account); err != nil {
				continue
			}
			if err := s.reconcileAllocationAccount(ctx, scheme, account); err != nil {
				continue
			}
		}
	}()
}

func (s *Service) reconcileAllocationAccount(ctx context.Context, scheme int64, account string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	now := s.now().Unix()
	rev, err := allocations.Current(ctx, q, scheme, now)
	if err != nil {
		return err
	}
	if rev.Config.Mode != "ratio" {
		return nil
	}
	if err := allocations.Sync(ctx, q, scheme, account, rev, now); err != nil {
		return err
	}
	return tx.Commit()
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
