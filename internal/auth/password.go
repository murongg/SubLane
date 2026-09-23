package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"

	"golang.org/x/crypto/argon2"
)

var (
	ErrCurrentPassword = errors.New("current_password_invalid")
	ErrPasswordChanged = errors.New("password_changed")
)

func (s *Service) ChangePassword(ctx context.Context, userID int64, current, password string) error {
	if !validPassword(password, 20) {
		return ErrInput
	}
	if !validPassword(current, 128) {
		return ErrCurrentPassword
	}
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer func() { <-s.hashSlots }()
	user, err := s.queries.GetPasswordUser(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !user.Enabled {
		return ErrCredentials
	}
	if err != nil {
		return err
	}
	if !verifyPassword(user.PasswordHash, current) {
		return ErrCurrentPassword
	}
	return s.replacePassword(ctx, user, password, "user.password", "user")
}

func (s *Service) ResetMemberPassword(ctx context.Context, id int64, password string) error {
	return s.resetPassword(ctx, id, RoleMember, password)
}

// RecoverAdministrator is only exposed by the local maintenance command, never an HTTP route.
func (s *Service) RecoverAdministrator(ctx context.Context, password string) error {
	ctx = audit.WithActor(ctx, audit.Actor{TenantID: 1, ID: 0, Username: "local-cli", Role: "admin", Source: "local"})
	return s.resetPassword(ctx, 1, RoleAdmin, password)
}

func (s *Service) resetPassword(ctx context.Context, id int64, role Role, password string) error {
	if !validPassword(password, 20) {
		return ErrInput
	}
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer func() { <-s.hashSlots }()
	user, err := s.queries.GetPasswordUser(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || err == nil && user.Role != string(role) {
		return ErrMemberNotFound
	}
	if err != nil {
		return err
	}
	action, resource := "member.password", "member"
	if role == RoleAdmin {
		action, resource = "user.recover", "user"
	}
	return s.replacePassword(ctx, user, password, action, resource)
}

func (s *Service) replacePassword(ctx context.Context, user db.GetPasswordUserRow, password, action, resource string) error {
	hash := hashPassword(password)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	n, err := q.ReplacePassword(ctx, db.ReplacePasswordParams{ID: user.ID, PreviousHash: user.PasswordHash, PasswordHash: hash, Enabled: user.Enabled})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrPasswordChanged
	}
	// Hash replacement and all-session revocation must either both commit or neither happen.
	if err := q.DeleteUserSessions(ctx, user.ID); err != nil {
		return err
	}
	if err := audit.Record(ctx, q, action, resource, audit.ID(user.ID)); err != nil {
		return err
	}
	return tx.Commit()
}

const hashPrefix = "$argon2id$v=19$m=19456,t=2,p=1$"

func hashPassword(password string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	return hashPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key)
}

func verifyPassword(encoded, password string) bool {
	// Accept only our bounded parameter set; persisted data must not choose arbitrary KDF costs.
	if !strings.HasPrefix(encoded, hashPrefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(encoded, hashPrefix), "$")
	if len(parts) != 2 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil || len(salt) != 16 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}
