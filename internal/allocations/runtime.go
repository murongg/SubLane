package allocations

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
)

func unix(n int64) time.Time { return time.Unix(n, 0) }
func Authorize(ctx context.Context, q *db.Queries, scheme, user, group, now int64) (Revision, error) {
	row, err := q.GetAllocationScheme(ctx, scheme)
	if err != nil || row.GroupID != group {
		return Revision{}, ErrUnavailable
	}
	allowed, err := q.CanUseAllocation(ctx, db.CanUseAllocationParams{ID: scheme, UserID: user})
	if err != nil {
		return Revision{}, err
	}
	if !allowed {
		return Revision{}, ErrUnavailable
	}
	rev, err := Current(ctx, q, scheme, now)
	if err != nil {
		return rev, err
	}
	for _, m := range rev.Config.Members {
		if m.UserID == user {
			return rev, nil
		}
	}
	return rev, ErrUnavailable
}
func KeyScheme(ctx context.Context, q *db.Queries, key, user, group, now int64) (int64, error) {
	id, err := q.GetKeyAllocation(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err = q.GetPoolAllocation(ctx, group); err == nil {
			return 0, ErrUnavailable
		} else if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	_, err = Authorize(ctx, q, id, user, group, now)
	return id, err
}
func rateFor(rev Revision, model string) (Rate, error) {
	for _, r := range rev.Config.Rates {
		if r.Model == model {
			return r, nil
		}
	}
	return Rate{}, ErrUnpriced
}
func Check(ctx context.Context, q *db.Queries, scheme, user, group int64, account, model string, now int64) (Revision, error) {
	rev, err := Authorize(ctx, q, scheme, user, group, now)
	if err != nil {
		return rev, err
	}
	if rev.Config.Mode != "tokens" {
		if _, err = rateFor(rev, model); err != nil {
			return rev, err
		}
	}
	count, err := q.AllocationMemberUnresolved(ctx, db.AllocationMemberUnresolvedParams{SchemeID: scheme, UserID: user, ResetAt: now})
	if err != nil {
		return rev, err
	}
	if count > 0 {
		return rev, ErrPending
	}
	if rev.Config.Mode == "ratio" {
		// Sync runs in a separate committed transaction so an exhausted admission cannot roll back reconciliation.
		windows, err := q.CurrentAccountAllocationWindows(ctx, db.CurrentAccountAllocationWindowsParams{SchemeID: scheme, AccountID: account, ResetAt: now})
		if err != nil {
			return rev, err
		}
		_, upstreamWindows, revision, err := readSnapshot(ctx, q, account, now)
		if err != nil {
			return rev, err
		}
		if len(windows) != len(upstreamWindows) {
			return rev, ErrSnapshot
		}
		unknown, err := q.AllocationUnassigned(ctx, scheme)
		if err != nil {
			return rev, err
		}
		if unknown > 0 {
			return rev, ErrPending
		}
		n, err := q.AllocationAccountUnfinished(ctx, account)
		if err != nil {
			return rev, err
		}
		if n > 0 {
			return rev, ErrPending
		}
		waiting, err := q.AllocationAccountAwaiting(ctx, db.AllocationAccountAwaitingParams{SchemeID: scheme, AccountID: account})
		if err != nil {
			return rev, err
		}
		if syncPaused(waiting, now) {
			return rev, ErrSync
		}
		for _, w := range windows {
			matches := false
			for _, upstream := range upstreamWindows {
				if w.Kind == upstream.kind && w.ResetAt == upstream.reset && w.AccountRevision == revision && upstream.points >= w.ObservedPoints {
					matches = true
				}
			}
			if !matches {
				return rev, ErrSnapshot
			}
			usage, err := q.AllocationWindowUsage(ctx, db.AllocationWindowUsageParams{WindowID: w.ID, UserID: 0})
			if err != nil {
				return rev, err
			}
			totalUsed, totalAllowance := int64(0), int64(0)
			for _, u := range usage {
				totalUsed += u.Used
				totalAllowance += u.Allowance
			}
			found := false
			for _, u := range usage {
				if u.UserID == user {
					found = true
					if u.Used >= u.Allowance {
						// Borrowing is deliberately pool-local: it can use only the
						// same physical account and window's unused member allowance.
						// Sticky conversations remain bound to this account.
						if !rev.Config.AllowIdleBorrow || totalUsed >= totalAllowance {
							return rev, ErrQuota
						}
					}
				}
			}
			if !found {
				return rev, ErrUnavailable
			}
		}
		return rev, nil
	}
	start, _ := Window(unix(now), rev.Config.Period)
	usage, err := q.AllocationMemberUsage(ctx, db.AllocationMemberUsageParams{SchemeID: scheme, UserID: user, WindowStart: start, Mode: rev.Config.Mode})
	if err != nil {
		return rev, err
	}
	for _, m := range rev.Config.Members {
		if m.UserID == user && usage.Used >= m.Limit {
			return rev, ErrQuota
		}
	}
	return rev, nil
}

const (
	ratioSyncGrace               = int64(120)
	ratioProvisionalRequestLimit = 32
)

func syncPaused(waiting db.AllocationAccountAwaitingRow, now int64) bool {
	// Bound both burst size and elapsed time; rounded or delayed percentages
	// never authorize unlimited uncharged traffic or erase existing debt.
	return waiting.Count >= ratioProvisionalRequestLimit || waiting.Count > 0 && now*1000-waiting.Oldest >= ratioSyncGrace*1000
}

type Request struct {
	ID                        string
	SchemeID, UserID, GroupID int64
	AccountID, Model          string
	StartedAt                 int64
}

func Begin(ctx context.Context, q *db.Queries, r Request, now int64) error {
	rev, err := Check(ctx, q, r.SchemeID, r.UserID, r.GroupID, r.AccountID, r.Model, now)
	if err != nil {
		return err
	}
	start, end := Window(unix(now), rev.Config.Period)
	if rev.Config.Mode == "ratio" {
		start, end = 0, 0
	}
	if err = q.BeginAllocationEntry(ctx, db.BeginAllocationEntryParams{RequestID: r.ID, SchemeID: r.SchemeID, RevisionID: rev.ID, UserID: r.UserID, AccountID: r.AccountID, Model: r.Model, Mode: rev.Config.Mode, WindowStart: start, ResetAt: end, StartedAt: r.StartedAt}); err != nil {
		return err
	}
	if rev.Config.Mode == "ratio" {
		windows, err := q.CurrentAccountAllocationWindows(ctx, db.CurrentAccountAllocationWindowsParams{SchemeID: r.SchemeID, AccountID: r.AccountID, ResetAt: now})
		if err != nil {
			return err
		}
		for _, w := range windows {
			if err = q.BeginAllocationDebit(ctx, db.BeginAllocationDebitParams{RequestID: r.ID, WindowID: w.ID}); err != nil {
				return err
			}
		}
	}
	return nil
}

type Completion struct {
	Input, Output, Cached int64
	Known                 bool
	Dispatched            bool
	Rejected              bool
}

func Finish(ctx context.Context, q *db.Queries, id string, c Completion, now int64, manual bool) error {
	e, err := q.GetAllocationEntry(ctx, id)
	if err != nil {
		return err
	}
	if !manual && e.State != "active" {
		return nil
	}
	if manual {
		if e.State == "settled" && e.Manual == 1 && e.InputTokens == c.Input && e.OutputTokens == c.Output && e.CachedTokens == c.Cached {
			return nil
		}
		if e.State != "pending" || c.Input < e.InputTokens || c.Output < e.OutputTokens {
			return ErrSettlement
		}
	}
	state := "settled"
	if c.Dispatched && !c.Rejected && !c.Known {
		state = "pending"
	}
	cost := c.Input + c.Output
	if c.Input < 0 || c.Output < 0 || c.Cached < 0 || c.Cached > c.Input || c.Input > 1_000_000_000 || c.Output > 1_000_000_000 {
		return ErrInput
	}
	if e.Mode != "tokens" {
		row, err := q.GetAllocationRevision(ctx, e.RevisionID)
		if err != nil {
			return err
		}
		rev, err := decodeRevision(row.ID, row.EffectiveAt, row.Config)
		if err != nil {
			return err
		}
		rate, err := rateFor(rev, e.Model)
		if err != nil {
			return err
		}
		cost, err = Cost(rate, c.Input, c.Output, c.Cached)
		if err != nil {
			return err
		}
	}
	if e.Mode == "ratio" && state != "pending" && c.Dispatched && !c.Rejected {
		state = "observed"
	}
	if e.Mode == "ratio" && state == "settled" {
		debits, err := q.GetRequestAllocationDebits(ctx, id)
		if err != nil {
			return err
		}
		for _, d := range debits {
			if err = q.SetAllocationDebit(ctx, db.SetAllocationDebitParams{RequestID: id, WindowID: d.WindowID, Points: 0}); err != nil {
				return err
			}
		}
	}
	return q.FinishAllocationEntry(ctx, db.FinishAllocationEntryParams{RequestID: id, FinishedAt: now, State: state, InputTokens: c.Input, OutputTokens: c.Output, CachedTokens: c.Cached, Cost: cost, Manual: bit(manual)})
}
func (s *Service) Refresh(ctx context.Context, id int64) error {
	q := db.New(s.conn)
	scheme, err := q.GetAllocationScheme(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	rev, err := Current(ctx, q, id, s.now().Unix())
	if err != nil {
		return err
	}
	if rev.Config.Mode != "ratio" {
		return nil
	}
	ids, err := q.ListGroupAccounts(ctx, scheme.GroupID)
	if err != nil {
		return err
	}
	healthy := false
	for _, account := range ids {
		err = s.syncAccount(ctx, id, account)
		if err == nil {
			healthy = true
			continue
		}
		if !errors.Is(err, ErrSnapshot) && !errors.Is(err, ErrPending) && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if !healthy {
		return ErrSnapshot
	}
	return nil
}

func (s *Service) syncAccount(ctx context.Context, id int64, account string) error {
	// Separate transactions isolate a stale peer without committing partial reconciliation.
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	now := s.now().Unix()
	rev, err := Current(ctx, q, id, now)
	if err != nil {
		return err
	}
	if rev.Config.Mode != "ratio" {
		return nil
	}
	if err = Sync(ctx, q, id, account, rev, now); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) Settle(ctx context.Context, scheme int64, id string, c Completion, points map[int64]int64) error {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	e, err := q.GetAllocationEntry(ctx, id)
	if err != nil || e.SchemeID != scheme {
		return ErrSettlement
	}
	c.Known = true
	c.Dispatched = true
	if e.Mode == "ratio" {
		if e.State != "pending" && e.State != "observed" && !(e.State == "settled" && e.Manual == 1) {
			return ErrSettlement
		}
		debits, err := q.GetRequestAllocationDebits(ctx, id)
		if err != nil {
			return err
		}
		if len(debits) != len(points) {
			return ErrInput
		}
		for _, d := range debits {
			p, ok := points[d.WindowID]
			if !ok || p < 0 || p > 10000 {
				return ErrInput
			}
			if d.Reconciled == 1 {
				if p != d.Points {
					return ErrSettlement
				}
				continue
			}
			window, err := q.GetAllocationWindow(ctx, d.WindowID)
			if err != nil {
				return err
			}
			if p > 10000-window.ObservedPoints {
				return ErrSettlement
			}
			if err = q.AdvanceManualAllocationWindow(ctx, db.AdvanceManualAllocationWindowParams{ID: d.WindowID, ObservedPoints: p}); err != nil {
				return err
			}
			if err = q.SetAllocationDebit(ctx, db.SetAllocationDebitParams{RequestID: id, WindowID: d.WindowID, Points: p}); err != nil {
				return err
			}
		}
		// Explicit points settle the original windows. Token metadata is never invented for ratio corrections.
		if err = q.FinishAllocationEntry(ctx, db.FinishAllocationEntryParams{RequestID: id, FinishedAt: s.now().UnixMilli(), State: "settled", InputTokens: e.InputTokens, OutputTokens: e.OutputTokens, CachedTokens: e.CachedTokens, Cost: e.Cost, Manual: 1}); err != nil {
			return err
		}
	} else if err = Finish(ctx, q, id, c, s.now().UnixMilli(), true); err != nil {
		return err
	}
	if err = audit.Record(ctx, q, "allocation.settle", "allocation", audit.ID(scheme)); err != nil {
		return err
	}
	return tx.Commit()
}

// Prune only closed periods; unknown usage and unreconciled percentage debt survive indefinitely.
func Prune(ctx context.Context, q *db.Queries, before int64) error {
	if err := q.PruneAllocations(ctx, before); err != nil {
		return err
	}
	return q.PruneAllocationWindows(ctx, before)
}
