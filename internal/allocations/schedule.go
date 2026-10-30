package allocations

import (
	"context"
	"database/sql"
	"errors"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
)

type Preview struct {
	WriteConditions     WriteConditions `json:"write_conditions"`
	ProposedEffectiveAt int64           `json:"proposed_effective_at"`
	ProposedAnchorAt    int64           `json:"proposed_accounting_anchor_at"`
}

func (s *Service) PreviewInitial(ctx context.Context, in SchemeInput) (Preview, error) {
	if !validPeriod(in.Config) {
		return Preview{}, ErrInput
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return Preview{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	if _, err := q.GetTenantGroup(ctx, db.GetTenantGroupParams{ID: in.GroupID, TenantID: s.tenantID}); err != nil {
		return Preview{}, ErrInput
	}
	if err := s.validateRecipients(ctx, q, in.GroupID, in.Config.Members); err != nil {
		return Preview{}, err
	}
	if _, err := s.applyPrices(ctx, q, in.GroupID, &in.Config, nil); err != nil {
		return Preview{}, err
	}
	if err := normalize(&in.Config); err != nil {
		return Preview{}, err
	}
	in.ExpectedEffectiveAt = nil
	at, err := s.creationTime(in, s.now().Unix())
	if err != nil {
		return Preview{}, err
	}
	return Preview{ProposedEffectiveAt: at, ProposedAnchorAt: at}, nil
}

func (s *Service) Preview(ctx context.Context, id int64, action string) (Preview, error) {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return Preview{}, err
	}
	defer tx.Rollback()
	state, err := s.readSchemeCore(ctx, db.New(tx), id, s.now().Unix())
	if err != nil {
		return Preview{}, err
	}
	if action != "schedule" && action != "reschedule" && action != "replace" {
		return Preview{}, ErrInput
	}
	at, err := s.targetTime(state, action, s.now().Unix())
	if err != nil {
		return Preview{}, err
	}
	anchor := state.AccountingAnchorAt
	if state.Next != nil {
		anchor = state.Next.AccountingAnchorAt
		if action == "reschedule" && (activeRevision(state) == nil || anchor != state.AccountingAnchorAt) {
			anchor = at
		}
	}
	if action == "replace" {
		anchor = at
	}
	return Preview{WriteConditions: state.WriteConditions, ProposedEffectiveAt: at, ProposedAnchorAt: anchor}, nil
}

func (s *Service) ChangeSchedule(ctx context.Context, id int64, action string, in Conditions) (Scheme, error) {
	if action != "cancel" && action != "reschedule" {
		return Scheme{}, ErrInput
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return Scheme{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	now := s.now().Unix()
	state, err := s.readSchemeCore(ctx, q, id, now)
	if err != nil {
		return Scheme{}, err
	}
	if err := checkVersions(in, state); err != nil {
		return Scheme{}, err
	}
	if state.Next == nil || state.Next.EffectiveAt <= now {
		return Scheme{}, ErrEffectiveConflict
	}
	if action == "cancel" && activeRevision(state) == nil {
		return Scheme{}, ErrInitialSchedule
	}
	var replacementID int64
	if action == "reschedule" {
		at, err := s.checkTime(in, state, action)
		if err != nil {
			return Scheme{}, err
		}
		anchor := state.Next.AccountingAnchorAt
		if activeRevision(state) == nil || anchor != state.AccountingAnchorAt {
			anchor = at
		}
		replacementID, err = appendRevision(ctx, q, id, at, anchor, state.Next.Config)
		if err != nil {
			return Scheme{}, err
		}
	}
	// Insert first; cleanup must target the old timestamp rather than all future rows.
	if err := q.DeleteAllocationPendingAt(ctx, db.DeleteAllocationPendingAtParams{SchemeID: id, EffectiveAt: state.Next.EffectiveAt, Now: now}); err != nil {
		return Scheme{}, err
	}
	next, err := q.NextAllocationRevision(ctx, db.NextAllocationRevisionParams{SchemeID: id, EffectiveAt: now})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Scheme{}, err
	}
	if action == "reschedule" {
		if err != nil || next.ID != replacementID {
			return Scheme{}, ErrStateConflict
		}
	} else if err == nil {
		return Scheme{}, ErrStateConflict
	}
	if err := audit.Record(ctx, q, "allocation.save", "allocation", audit.ID(id)); err != nil {
		return Scheme{}, err
	}
	if action == "reschedule" {
		if _, err := s.checkTime(in, state, action); err != nil {
			return Scheme{}, err
		}
	}
	guard := ""
	if action == "reschedule" {
		guard = action
	}
	return s.finishChange(ctx, tx, q, state, in, guard)
}
