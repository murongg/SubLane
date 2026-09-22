package allocations

import (
	"context"
	"errors"
	"strconv"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
)

type Balance struct {
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	Mode         string `json:"mode"`
	Limit        int64  `json:"limit"`
	Used         int64  `json:"used"`
	Borrowed     int64  `json:"borrowed"`
	Tokens       int64  `json:"tokens"`
	Pending      int64  `json:"pending"`
	ResetAt      int64  `json:"reset_at"`
	WindowID     int64  `json:"window_id"`
	WindowKind   string `json:"window_kind"`
	AccountID    string `json:"account_id,omitempty"`
	AccountLabel string `json:"account_label,omitempty"`
	Syncing      int64  `json:"syncing"`
	SyncPaused   bool   `json:"sync_paused"`
}
type Debit struct {
	WindowID   int64  `json:"window_id"`
	Kind       string `json:"kind"`
	ResetAt    int64  `json:"reset_at"`
	Points     int64  `json:"points"`
	Reconciled bool   `json:"reconciled"`
}
type Pending struct {
	RequestID string  `json:"request_id"`
	UserID    int64   `json:"user_id"`
	Mode      string  `json:"mode"`
	Model     string  `json:"model"`
	State     string  `json:"state"`
	Input     int64   `json:"input"`
	Output    int64   `json:"output"`
	Cached    int64   `json:"cached"`
	Debits    []Debit `json:"debits"`
	Automatic bool    `json:"automatic"`
}
type Detail struct {
	Scheme
	Balances   []Balance `json:"balances"`
	Pending    []Pending `json:"pending"`
	Unassigned int64     `json:"unassigned"`
	Available  bool      `json:"available"`
}

func (s *Service) Detail(ctx context.Context, id, user int64) (Detail, error) {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	now := s.now().Unix()
	scheme, err := readScheme(ctx, q, id, now)
	if err != nil {
		return Detail{}, err
	}
	out := Detail{Scheme: scheme, Balances: []Balance{}, Pending: []Pending{}, Available: scheme.Enabled && scheme.EffectiveAt <= now}
	if user > 0 {
		allowed, err := q.CanUseAllocation(ctx, db.CanUseAllocationParams{ID: id, UserID: user})
		if err != nil {
			return out, err
		}
		out.Available = out.Available && allowed
		found := false
		for _, m := range scheme.Config.Members {
			if m.UserID == user {
				found = true
			}
		}
		if !found {
			return out, ErrUnavailable
		}
	}
	pending, err := q.ListAllocationPending(ctx, db.ListAllocationPendingParams{SchemeID: id, UserID: user})
	if err != nil {
		return out, err
	}
	for _, e := range pending {
		if user > 0 && user != e.UserID {
			continue
		}
		p := Pending{RequestID: e.RequestID, UserID: e.UserID, Mode: e.Mode, Model: e.Model, State: e.State, Input: e.InputTokens, Output: e.OutputTokens, Cached: e.CachedTokens, Debits: []Debit{}}
		p.Automatic = e.State == "observed" && e.FinishedAt > (now-ratioSyncGrace)*1000
		ds, err := q.GetRequestAllocationDebits(ctx, e.RequestID)
		if err != nil {
			return out, err
		}
		for _, d := range ds {
			if d.Reconciled == 0 && d.ResetAt <= now {
				p.Automatic = false
			}
			p.Debits = append(p.Debits, Debit{WindowID: d.WindowID, Kind: d.Kind, ResetAt: d.ResetAt, Points: d.Points, Reconciled: d.Reconciled == 1})
		}
		out.Pending = append(out.Pending, p)
	}
	if scheme.Config.Mode == "ratio" {
		windows, err := q.ListAllocationWindows(ctx, id)
		if err != nil {
			return out, err
		}
		labels := map[string]string{}
		for _, w := range windows {
			if labels[w.AccountID] == "" {
				labels[w.AccountID] = strconv.Itoa(len(labels) + 1)
			}
		}
		for _, w := range windows {
			if w.ResetAt <= now {
				continue
			}
			usage, err := q.AllocationWindowUsage(ctx, db.AllocationWindowUsageParams{WindowID: w.ID, UserID: user})
			if err != nil {
				return out, err
			}
			waiting, err := q.AllocationAccountAwaiting(ctx, db.AllocationAccountAwaitingParams{SchemeID: id, AccountID: w.AccountID})
			if err != nil {
				return out, err
			}
			_, _, _, snapshotErr := readSnapshot(ctx, q, w.AccountID, now)
			debits, err := q.ListAllocationDebits(ctx, w.ID)
			if err != nil {
				return out, err
			}
			for _, u := range usage {
				if user > 0 && u.UserID != user {
					continue
				}
				member, err := q.GetMember(ctx, u.UserID)
				if err != nil {
					return out, err
				}
				n, err := q.AllocationMemberUnresolved(ctx, db.AllocationMemberUnresolvedParams{SchemeID: id, UserID: u.UserID, ResetAt: now})
				if err != nil {
					return out, err
				}
				syncing := int64(0)
				for _, d := range debits {
					if d.UserID == u.UserID && d.State == "observed" {
						syncing++
					}
				}
				tokens, e := q.AllocationWindowTokens(ctx, db.AllocationWindowTokensParams{WindowID: w.ID, UserID: u.UserID})
				if e != nil {
					return out, e
				}
				borrowed := int64(0)
				if u.Used > u.Allowance {
					borrowed = u.Used - u.Allowance
				}
				balance := Balance{Tokens: tokens, UserID: u.UserID, Username: member.Username, Mode: "ratio", Limit: u.Allowance, Used: u.Used, Borrowed: borrowed, Pending: n, ResetAt: w.ResetAt, WindowID: w.ID, WindowKind: w.Kind}
				balance.AccountLabel, balance.Syncing, balance.SyncPaused = labels[w.AccountID], syncing, syncPaused(waiting, now) || snapshotErr != nil || w.Unassigned > 0
				if user == 0 {
					balance.AccountID = w.AccountID
				}
				out.Balances = append(out.Balances, balance)
			}
		}
	} else {
		start, end := Window(unix(now), scheme.Config.Period)
		for _, m := range scheme.Config.Members {
			if user > 0 && m.UserID != user {
				continue
			}
			u, err := q.AllocationMemberUsage(ctx, db.AllocationMemberUsageParams{SchemeID: id, UserID: m.UserID, WindowStart: start, Mode: scheme.Config.Mode})
			if err != nil {
				return out, err
			}
			member, err := q.GetMember(ctx, m.UserID)
			if err != nil {
				return out, err
			}
			n, err := q.AllocationMemberPending(ctx, db.AllocationMemberPendingParams{SchemeID: id, UserID: m.UserID})
			if err != nil {
				return out, err
			}
			out.Balances = append(out.Balances, Balance{UserID: m.UserID, Username: member.Username, Mode: scheme.Config.Mode, Limit: m.Limit, Used: u.Used, Tokens: u.Tokens, Pending: n, ResetAt: end})
		}
	}
	out.Unassigned, err = q.AllocationUnassigned(ctx, id)
	if err != nil {
		return out, err
	}
	if user > 0 {
		out.Config.Members = ownShares(out.Config.Members, user)
		if out.Next != nil {
			out.Next.Config.Members = ownShares(out.Next.Config.Members, user)
		}
	}
	return out, tx.Commit()
}
func ownShares(all []Share, user int64) []Share {
	out := []Share{}
	for _, s := range all {
		if s.UserID == user {
			out = append(out, s)
		}
	}
	return out
}
func (s *Service) Own(ctx context.Context, user int64) ([]Detail, error) {
	q := db.New(s.conn)
	rows, err := q.ListAllocationSchemes(ctx)
	if err != nil {
		return nil, err
	}
	out := []Detail{}
	for _, row := range rows {
		ids, err := q.ListAllocationTeamMembers(ctx, row.TeamID)
		if err != nil {
			return nil, err
		}
		found := false
		for _, id := range ids {
			if id == user {
				found = true
			}
		}
		if !found {
			continue
		}
		d, err := s.Detail(ctx, row.ID, user)
		if errors.Is(err, ErrUnavailable) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}
func (s *Service) ReserveUnassigned(ctx context.Context, scheme, expected int64) error {
	if expected <= 0 {
		return ErrInput
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	current, err := q.AllocationUnassigned(ctx, scheme)
	if err != nil {
		return err
	}
	if current != expected {
		return ErrSettlement
	}
	if err = q.ClearAllocationUnassigned(ctx, scheme); err != nil {
		return err
	}
	if err = audit.Record(ctx, q, "allocation.reconcile", "allocation", audit.ID(scheme)); err != nil {
		return err
	}
	return tx.Commit()
}
