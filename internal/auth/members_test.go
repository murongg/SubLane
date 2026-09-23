package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestMemberLifecycleAndSessionIsolation(t *testing.T) {
	s, connection := fixture(t)
	ctx := context.Background()
	owner, err := s.Setup(ctx, "owner-test", syntheticPassword, "Synthetic workspace")
	if err != nil {
		t.Fatal(err)
	}
	if owner.User.Role != "admin" || owner.User.ID != 1 {
		t.Fatalf("wrong owner identity: %+v", owner.User)
	}
	member, err := s.CreateMember(ctx, "Member-Test", "member pass 42")
	if err != nil {
		t.Fatal(err)
	}
	if member.Role != "member" || member.ID <= 1 || !member.Enabled || member.Username != "member-test" {
		t.Fatalf("wrong member: %+v", member)
	}
	var workspaceRole string
	if err := connection.QueryRow("SELECT role FROM memberships WHERE tenant_id=1 AND user_id=?", member.ID).Scan(&workspaceRole); err != nil || workspaceRole != "member" {
		t.Fatalf("new member was not enrolled in the initial workspace: role=%q err=%v", workspaceRole, err)
	}
	if _, err := s.CreateMember(ctx, "MEMBER-TEST", "member pass 42"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate accepted: %v", err)
	}
	if _, err := s.CreateMember(ctx, "invalid name", "member pass 42"); !errors.Is(err, ErrInput) {
		t.Fatalf("bad username accepted: %v", err)
	}
	var first, last Session
	for i := 0; i < 6; i++ {
		last, err = s.Login(ctx, "member-test", "member pass 42")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = last
		}
	}
	if state, err := s.State(ctx, first.Token); err != nil || state.User != nil {
		t.Fatal("member session cap failed", err)
	}
	if state, err := s.State(ctx, owner.Token); err != nil || state.User == nil || state.User.Role != "admin" {
		t.Fatal("member logins evicted administrator", err)
	}
	if _, err := s.SetMemberEnabled(ctx, 1, false); !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("owner status can be changed: %v", err)
	}
	if _, err := s.SetMemberEnabled(ctx, member.ID, false); err != nil {
		t.Fatal(err)
	}
	if state, err := s.State(ctx, last.Token); err != nil || state.User != nil {
		t.Fatal("disabled session still works", err)
	}
	if _, err := s.Login(ctx, "member-test", "member pass 42"); !errors.Is(err, ErrCredentials) {
		t.Fatalf("disabled account logged in: %v", err)
	}
	if _, err := s.SetMemberEnabled(ctx, member.ID, true); err != nil {
		t.Fatal(err)
	}
	if state, err := s.State(ctx, last.Token); err != nil || state.User != nil {
		t.Fatal("re-enable revived a revoked session", err)
	}
	restored, err := s.Login(ctx, "member-test", "member pass 42")
	if err != nil || restored.User.Role != "member" {
		t.Fatalf("re-enabled member cannot log in: %v", err)
	}
}

func TestMembersArePaginatedAndExcludeAdministrator(t *testing.T) {
	s, db := fixture(t)
	ctx := context.Background()
	if _, err := s.Setup(ctx, "owner-test", syntheticPassword, "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 51; i++ {
		if _, err := db.Exec("INSERT INTO users(username, role, password_hash, created_at) VALUES (?, 'member', 'synthetic-hash', 1)", fmt.Sprintf("member-%03d", i)); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.ListMembers(ctx, 0)
	if err != nil || len(page.Members) != 50 || page.NextCursor == 0 {
		t.Fatalf("bad first page: %+v %v", page, err)
	}
	next, err := s.ListMembers(ctx, page.NextCursor)
	if err != nil || len(next.Members) != 1 || next.NextCursor != 0 || next.Members[0].ID >= page.NextCursor {
		t.Fatalf("bad next page: %+v %v", next, err)
	}
	for _, member := range page.Members {
		if member.Role != "member" {
			t.Fatal("administrator leaked into member list")
		}
	}
}

func TestSessionCreationRechecksDisabledUser(t *testing.T) {
	s, db := fixture(t)
	ctx := context.Background()
	if _, err := s.Setup(ctx, "owner-test", syntheticPassword, "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	member, err := s.CreateMember(ctx, "member-test", "member pass 42")
	if err != nil {
		t.Fatal(err)
	}
	verified, err := s.queries.GetLoginUser(ctx, member.Username)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetMemberEnabled(ctx, member.ID, false); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := s.createSession(ctx, tx, member.User, verified.PasswordHash); !errors.Is(err, ErrCredentials) {
		t.Fatalf("session issued after disable: %v", err)
	}
}
