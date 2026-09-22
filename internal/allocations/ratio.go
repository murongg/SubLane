package allocations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"

	"github.com/murongg/SubLane/internal/storage/db"
)

type snapshot struct {
	UpdatedAt     int64 `json:"updated_at"`
	ReadStartedAt int64 `json:"read_started_at"`
	Limits        []struct {
		Name    string `json:"name"`
		Windows []struct {
			Kind  string   `json:"kind"`
			Used  *float64 `json:"used_percent"`
			Reset *int64   `json:"reset_at"`
		} `json:"windows"`
	} `json:"limits"`
}
type quotaWindow struct {
	kind          string
	points, reset int64
}

func readSnapshot(ctx context.Context, q *db.Queries, account string, now int64) (snapshot, []quotaWindow, int64, error) {
	var s snapshot
	r, err := q.GetAccountUsage(ctx, account)
	if err != nil {
		return s, nil, 0, err
	}
	if json.Unmarshal(r.Snapshot, &s) != nil || s.UpdatedAt <= 0 || s.UpdatedAt > now || now-s.UpdatedAt >= 120 {
		return s, nil, 0, ErrSnapshot
	}
	windows := []quotaWindow{}
	for _, limit := range s.Limits {
		if limit.Name != "" {
			continue
		}
		for _, w := range limit.Windows {
			if w.Used == nil || w.Reset == nil || *w.Reset <= now || *w.Used < 0 || math.IsNaN(*w.Used) || math.IsInf(*w.Used, 0) {
				return s, nil, 0, ErrSnapshot
			}
			windows = append(windows, quotaWindow{w.Kind, int64(math.Round(math.Min(100, *w.Used) * 100)), *w.Reset})
		}
	}
	if len(windows) == 0 {
		return s, nil, 0, ErrSnapshot
	}
	return s, windows, r.Revision, nil
}
func nextEffective(ctx context.Context, q *db.Queries, group int64, c Config, now int64) (int64, error) {
	if c.Mode != "ratio" {
		_, end := Window(unix(now), c.Period)
		return end, nil
	}
	ids, err := q.ListGroupAccounts(ctx, group)
	if err != nil {
		return 0, err
	}
	var end int64
	for _, id := range ids {
		_, windows, _, err := readSnapshot(ctx, q, id, now)
		if err != nil {
			return 0, err
		}
		for _, w := range windows {
			end = max(end, w.reset)
		}
	}
	if end <= now {
		return 0, ErrSnapshot
	}
	return end, nil
}

// Reconcile uses only intervals with no unfinished requests and an observation
// that started after all included completions. Poll latency must not charge a future request.
func Sync(ctx context.Context, q *db.Queries, scheme int64, account string, rev Revision, now int64) error {
	snap, windows, accountRevision, err := readSnapshot(ctx, q, account, now)
	if err != nil {
		return err
	}
	if err = q.MarkExpiredAllocationEntries(ctx, db.MarkExpiredAllocationEntriesParams{SchemeID: scheme, ResetAt: now}); err != nil {
		return err
	}
	for _, w := range windows {
		old, err := q.FindAllocationWindow(ctx, db.FindAllocationWindowParams{SchemeID: scheme, AccountID: account, Kind: w.kind, ResetAt: w.reset})
		if errors.Is(err, sql.ErrNoRows) {
			id, err := q.CreateAllocationWindow(ctx, db.CreateAllocationWindowParams{SchemeID: scheme, AccountID: account, Kind: w.kind, ResetAt: w.reset, AccountRevision: accountRevision, ObservedAt: snap.UpdatedAt, ObservedPoints: w.points, BaselinePoints: w.points})
			if err != nil {
				return err
			}
			for _, m := range rev.Config.Members {
				if err = q.AddAllocationWindowMember(ctx, db.AddAllocationWindowMemberParams{WindowID: id, UserID: m.UserID, Allowance: (10000 - w.points) * m.Limit / 10000}); err != nil {
					return err
				}
			}
			continue
		}
		if err != nil {
			return err
		}
		if w.points < old.ObservedPoints {
			return ErrSnapshot
		}
		if old.AccountRevision != accountRevision {
			// Account identity is immutable. A fresh lifecycle snapshot may revalidate
			// the same window, but must never reset its allowance or outstanding debt.
			if err = q.RevalidateAllocationWindow(ctx, db.RevalidateAllocationWindowParams{ID: old.ID, AccountRevision: accountRevision}); err != nil {
				return err
			}
		}
		if snap.UpdatedAt <= old.ObservedAt {
			continue
		}
		unfinished, err := q.AllocationAccountUnfinished(ctx, account)
		if err != nil {
			return err
		}
		if unfinished > 0 {
			continue
		}
		debits, err := q.ListAllocationDebits(ctx, old.ID)
		if err != nil {
			return err
		}
		weights := make([]int64, len(debits))
		safe := true
		for i, d := range debits {
			weights[i] = d.Cost
			if d.State != "observed" || snap.ReadStartedAt == 0 || d.FinishedAt >= snap.ReadStartedAt {
				safe = false
			}
		}
		if !safe {
			continue
		}
		delta := w.points - old.ObservedPoints
		// No percentage change is not proof of zero cost: retain the request debt until a later observation.
		if delta == 0 {
			continue
		}
		unassigned := int64(0)
		if len(debits) == 0 {
			unassigned = delta
		} else {
			shares, err := Distribute(delta, weights)
			if err != nil {
				return err
			}
			for i, d := range debits {
				if err = q.SetAllocationDebit(ctx, db.SetAllocationDebitParams{RequestID: d.RequestID, WindowID: old.ID, Points: shares[i]}); err != nil {
					return err
				}
			}
		}
		if err = q.ObserveAllocationWindow(ctx, db.ObserveAllocationWindowParams{ID: old.ID, ObservedAt: snap.UpdatedAt, ObservedPoints: w.points, Unassigned: unassigned}); err != nil {
			return err
		}
	}
	return q.SettleReconciledAllocationEntries(ctx, scheme)
}
