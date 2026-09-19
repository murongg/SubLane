package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPasswordChangeRevokesSessionsAndPreservesOtherUsers(t *testing.T) {
	s, connection := fixture(t)
	ctx := context.Background()
	owner, err := s.Setup(ctx, "owner-test", syntheticPassword)
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.CreateMember(ctx, "member-test", syntheticPassword)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Login(ctx, member.Username, syntheticPassword)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Login(ctx, member.Username, syntheticPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ChangePassword(ctx, member.ID, "wrong password", "new password 42"); !errors.Is(err, ErrCurrentPassword) {
		t.Fatal("wrong password accepted", err)
	}
	for _, password := range []string{"short", strings.Repeat("x", 21)} {
		if err := s.ChangePassword(ctx, member.ID, syntheticPassword, password); !errors.Is(err, ErrInput) {
			t.Fatal("invalid length accepted", err)
		}
	}
	if err := s.ChangePassword(ctx, member.ID, syntheticPassword, strings.Repeat("界", 20)); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{first.Token, second.Token} {
		state, err := s.State(ctx, token)
		if err != nil || state.User != nil {
			t.Fatal("password change left a session alive", err)
		}
	}
	state, err := s.State(ctx, owner.Token)
	if err != nil || state.User == nil {
		t.Fatal("another user was signed out", err)
	}
	if _, err := s.Login(ctx, member.Username, syntheticPassword); !errors.Is(err, ErrCredentials) {
		t.Fatal("old password still works", err)
	}
	if _, err := s.Login(ctx, member.Username, strings.Repeat("界", 20)); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := connection.QueryRow("SELECT password_hash FROM users WHERE id=?", member.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, hashPrefix) {
		t.Fatal("password is not hashed")
	}
}

func TestResetPasswordBoundariesAndRollback(t *testing.T) {
	s, connection := fixture(t)
	ctx := context.Background()
	if err := s.RecoverAdministrator(ctx, "new password 42"); err == nil {
		t.Fatal("recovery created a missing administrator")
	}
	owner, err := s.Setup(ctx, "owner-test", syntheticPassword)
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.CreateMember(ctx, "member-test", syntheticPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ResetMemberPassword(ctx, 1, "new password 42"); !errors.Is(err, ErrMemberNotFound) {
		t.Fatal("member reset reached administrator", err)
	}
	if _, err := s.SetMemberEnabled(ctx, member.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetMemberPassword(ctx, member.ID, "new password 42"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Login(ctx, member.Username, "new password 42"); !errors.Is(err, ErrCredentials) {
		t.Fatal("reset reenabled member", err)
	}
	if _, err := connection.Exec("CREATE TRIGGER fail_session_delete BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT,'synthetic failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverAdministrator(ctx, "new password 42"); err == nil {
		t.Fatal("reset succeeded without session revocation")
	}
	if _, err := connection.Exec("DROP TRIGGER fail_session_delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Login(ctx, "owner-test", syntheticPassword); err != nil {
		t.Fatal("failed reset changed password", err)
	}
	if err := s.RecoverAdministrator(ctx, "new password 42"); err != nil {
		t.Fatal(err)
	}
	state, err := s.State(ctx, owner.Token)
	if err != nil || state.User != nil {
		t.Fatal("recovery retained old session", err)
	}
	if _, err := s.Login(ctx, "owner-test", "new password 42"); err != nil {
		t.Fatal(err)
	}
}

func TestSessionCreationRejectsHashVerifiedBeforePasswordReset(t *testing.T) {
	s, connection := fixture(t)
	ctx := context.Background()
	owner, err := s.Setup(ctx, "owner-test", syntheticPassword)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.queries.GetLoginUser(ctx, "owner-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverAdministrator(ctx, "new password 42"); err != nil {
		t.Fatal(err)
	}
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := s.createSession(ctx, tx, owner.User, before.PasswordHash); !errors.Is(err, ErrCredentials) {
		t.Fatal("stale verified password created session", err)
	}
}
