package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/tenants"
)

var ErrInvitationInvalid = errors.New("invitation_invalid")

const invitationTTL = 7 * 24 * time.Hour

type Invitation struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Service) CreateInvitation(ctx context.Context, tenantID, actorID int64) (Invitation, error) {
	if tenantID <= 0 || actorID <= 0 {
		return Invitation{}, ErrInput
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Invitation{}, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	allowed, err := queries.CanManageTenant(ctx, db.CanManageTenantParams{TenantID: tenantID, ActorID: actorID})
	if err != nil {
		return Invitation{}, err
	}
	if !allowed {
		return Invitation{}, tenants.ErrForbidden
	}
	now := s.now()
	invitation := Invitation{Token: newToken(), ExpiresAt: now.Add(invitationTTL)}
	digest := sha256.Sum256([]byte(invitation.Token))
	id, err := queries.CreateInvitation(ctx, db.CreateInvitationParams{
		TokenHash: digest[:], TenantID: tenantID, CreatedBy: actorID,
		CreatedAt: now.Unix(), ExpiresAt: invitation.ExpiresAt.Unix(),
	})
	if err != nil {
		return Invitation{}, err
	}
	if err := audit.Record(ctx, queries, "invitation.create", "invitation", audit.ID(id)); err != nil {
		return Invitation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Invitation{}, err
	}
	return invitation, nil
}

func (s *Service) RegisterInvitation(ctx context.Context, tenantID int64, token, username, password string) (Session, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !validCredentials(username, password, 20) {
		return Session{}, ErrInput
	}
	if tenantID <= 0 || !validToken(token) {
		return Session{}, ErrInvitationInvalid
	}
	digest := sha256.Sum256([]byte(token))
	lookup := db.GetUsableInvitationParams{TokenHash: digest[:], TenantID: tenantID, Now: s.now().Unix()}
	if _, err := s.queries.GetUsableInvitation(ctx, lookup); errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrInvitationInvalid
	} else if err != nil {
		return Session{}, err
	}
	if err := s.acquire(ctx); err != nil {
		return Session{}, err
	}
	defer func() { <-s.hashSlots }()
	hash := hashPassword(password)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	now := s.now().Unix()
	lookup.Now = now
	// Recheck after hashing so a concurrent registration or expiry cannot reuse the link.
	id, err := queries.GetUsableInvitation(ctx, lookup)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrInvitationInvalid
	}
	if err != nil {
		return Session{}, err
	}
	row, err := queries.CreateMember(ctx, db.CreateMemberParams{Username: username, PasswordHash: hash, CreatedAt: now})
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrUsernameTaken
	}
	if err != nil {
		return Session{}, err
	}
	if err := tenants.AddNewMember(ctx, tx, tenantID, row.ID, now); err != nil {
		return Session{}, err
	}
	used, err := queries.ConsumeInvitation(ctx, db.ConsumeInvitationParams{UsedAt: &now, UsedBy: &row.ID, ID: id})
	if err != nil {
		return Session{}, err
	}
	if used != 1 {
		return Session{}, ErrInvitationInvalid
	}
	session, err := s.createSession(ctx, tx, User{ID: row.ID}, hash)
	if err != nil {
		return Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return Session{}, err
	}
	return session, nil
}
