package allocations

import (
	"context"
	"errors"

	"github.com/murongg/SubLane/internal/storage/db"
)

type Balance struct {
	UserID         int64  `json:"user_id"`
	Username       string `json:"username"`
	Mode           string `json:"mode"`
	Limit          int64  `json:"limit"`
	Used           int64  `json:"used"`
	Tokens         int64  `json:"tokens"`
	Pending        int64  `json:"pending"`
	PendingCurrent int64  `json:"pending_current"`
	InFlight       int64  `json:"in_flight"`
	Reserved       int64  `json:"reserved"`
	AdmissionRoom  int64  `json:"admission_room"`
	Admission      string `json:"admission"`
	ResetAt        int64  `json:"reset_at"`
}

type Pending struct {
	RequestID string `json:"request_id"`
	UserID    int64  `json:"user_id"`
	Mode      string `json:"mode"`
	Model     string `json:"model"`
	State     string `json:"state"`
	Input     int64  `json:"input"`
	Output    int64  `json:"output"`
	Cached    int64  `json:"cached"`
}

type Detail struct {
	Scheme
	Balances  []Balance `json:"balances"`
	Pending   []Pending `json:"pending"`
	Available bool      `json:"available"`
}

func (s *Service) Detail(ctx context.Context, id, user int64) (Detail, error) {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	now := s.now().Unix()
	scheme, err := readScheme(ctx, q, s.tenantID, id, now)
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
		out.Pending = append(out.Pending, Pending{RequestID: e.RequestID, UserID: e.UserID, Mode: e.Mode, Model: e.Model, State: e.State, Input: e.InputTokens, Output: e.OutputTokens, Cached: e.CachedTokens})
	}
	start, end := effectiveWindow(scheme.Revision, now, s.location())
	mode := entryMode(scheme.Config)
	for _, m := range scheme.Config.Members {
		if user > 0 && m.UserID != user {
			continue
		}
		u, err := q.AllocationMemberUsage(ctx, db.AllocationMemberUsageParams{SchemeID: id, UserID: m.UserID, StartedFrom: start, StartedTo: end, Mode: mode})
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
		limit := m.Limit
		if scheme.Config.Mode == "ratio" {
			limit = ratioLimit(scheme.Config, m.Limit)
		}
		exposure, err := q.AllocationMemberExposure(ctx, db.AllocationMemberExposureParams{SchemeID: id, UserID: m.UserID, StartedFrom: start, StartedTo: end, Mode: mode})
		if err != nil {
			return out, err
		}
		risk := currentRiskBudget(u.Used, limit, exposure.Active, exposure.Pending)
		admission := "active"
		if u.Used >= limit {
			admission = "exhausted"
		} else if risk.limited {
			admission = "risk_limited"
		}
		out.Balances = append(out.Balances, Balance{
			UserID: m.UserID, Username: member.Username, Mode: mode, Limit: limit, Used: u.Used,
			Tokens: u.Tokens, Pending: n, PendingCurrent: exposure.Pending, InFlight: exposure.Active,
			Reserved: risk.reserved, AdmissionRoom: risk.room, Admission: admission, ResetAt: end,
		})
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
	rows, err := q.ListAllocationSchemes(ctx, s.tenantID)
	if err != nil {
		return nil, err
	}
	out := []Detail{}
	for _, row := range rows {
		allowed, err := q.CanUseAllocation(ctx, db.CanUseAllocationParams{ID: row.ID, UserID: user})
		if err != nil {
			return nil, err
		}
		if !allowed {
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
