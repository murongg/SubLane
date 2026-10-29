package tenants

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
)

func (s *Service) RemoveMember(ctx context.Context, actorID, tenantID, userID int64) error {
	if actorID == userID {
		return ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	allowed, err := q.CanManageTenant(ctx, db.CanManageTenantParams{TenantID: tenantID, ActorID: actorID})
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	actor, err := q.GetTenantMember(ctx, db.GetTenantMemberParams{TenantID: tenantID, UserID: actorID})
	if err != nil {
		return err
	}
	member, err := q.GetTenantMember(ctx, db.GetTenantMemberParams{TenantID: tenantID, UserID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if member.RemovedAt != nil {
		return ErrNotFound
	}
	if member.Role == string(RoleOwner) || member.Role == string(RoleAdmin) && actor.Role != string(RoleOwner) {
		return ErrForbidden
	}
	now := time.Now().Unix()
	// Keep the membership for historical allowance reads; the CHECK constraint forbids re-enabling it.
	if err := q.RemoveTenantMember(ctx, db.RemoveTenantMemberParams{TenantID: tenantID, UserID: userID, Now: &now}); err != nil {
		return err
	}
	if err := q.ClearMemberGroups(ctx, db.ClearMemberGroupsParams{TenantID: tenantID, UserID: userID}); err != nil {
		return err
	}
	// Revocation is permanent so explicitly rejoining cannot revive an old key.
	if err := q.RevokeTenantMemberKeys(ctx, db.RevokeTenantMemberKeysParams{TenantID: tenantID, UserID: userID, Now: &now}); err != nil {
		return err
	}
	if err := audit.Record(ctx, q, "member.remove", "member", audit.ID(userID)); err != nil {
		return err
	}
	return tx.Commit()
}
