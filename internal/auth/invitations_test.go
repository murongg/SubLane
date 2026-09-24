package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInvitationRegistersOneMemberInItsWorkspace(t *testing.T) {
	s, connection := fixture(t)
	ctx := context.Background()
	s.now = func() time.Time { return time.Unix(1900000000, 0) }
	if _, err := s.Setup(ctx, "owner-test", syntheticPassword, "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	invitation, err := s.CreateInvitation(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !validToken(invitation.Token) || invitation.ExpiresAt.Sub(s.now()) != 7*24*time.Hour {
		t.Fatalf("invalid invitation metadata: %+v", invitation)
	}
	var stored []byte
	if err := connection.QueryRowContext(ctx, "SELECT token_hash FROM invitations").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if string(stored) == invitation.Token {
		t.Fatal("raw invitation secret was persisted")
	}
	if _, err := s.RegisterInvitation(ctx, 1, invitation.Token, "bad name", syntheticPassword); !errors.Is(err, ErrInput) {
		t.Fatalf("invalid account accepted: %v", err)
	}
	session, err := s.RegisterInvitation(ctx, 1, invitation.Token, "invited-test", syntheticPassword)
	if err != nil || session.User.Username != "invited-test" || session.User.Role != RoleMember {
		t.Fatalf("registration failed: %+v %v", session, err)
	}
	var role string
	if err := connection.QueryRowContext(ctx, "SELECT role FROM memberships WHERE user_id=? AND tenant_id=1", session.User.ID).Scan(&role); err != nil || role != "member" {
		t.Fatalf("member missing from invited workspace: %q %v", role, err)
	}
	if _, err := s.RegisterInvitation(ctx, 1, invitation.Token, "second-test", syntheticPassword); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("used invitation accepted: %v", err)
	}
}

func TestInvitationExpiryAndUsernameConflictDoNotConsumeIt(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	s.now = func() time.Time { return time.Unix(1900000000, 0) }
	if _, err := s.Setup(ctx, "owner-test", syntheticPassword, "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	invitation, err := s.CreateInvitation(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RegisterInvitation(ctx, 1, invitation.Token, "owner-test", syntheticPassword); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("existing username result: %v", err)
	}
	if _, err := s.RegisterInvitation(ctx, 2, invitation.Token, "new-test", syntheticPassword); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("wrong workspace accepted: %v", err)
	}
	s.now = func() time.Time { return invitation.ExpiresAt }
	if _, err := s.RegisterInvitation(ctx, 1, invitation.Token, "new-test", syntheticPassword); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("expired invitation accepted: %v", err)
	}
}
