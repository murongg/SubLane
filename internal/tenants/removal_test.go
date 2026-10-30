package tenants

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/storage/db"
)

func removalFixture(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	conn, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	_, err = conn.Exec(`
	INSERT INTO users(id,username,role,password_hash,created_at) VALUES
	 (1,'synthetic-owner','admin','fake-hash',1),(2,'synthetic-member','member','fake-hash',1),
	 (3,'synthetic-other-owner','member','fake-hash',1),(4,'synthetic-admin','member','fake-hash',1),
	 (5,'synthetic-peer','member','fake-hash',1);
	INSERT INTO tenants(id,name,owner_user_id,created_at) VALUES(1,'Synthetic alpha',1,1),(2,'Synthetic beta',3,1);
	INSERT INTO memberships(tenant_id,user_id,role,created_at) VALUES
	 (1,1,'owner',1),(1,2,'member',1),(1,4,'admin',1),(1,5,'member',1),(2,3,'owner',1),(2,2,'member',1);
	INSERT INTO account_groups(id,tenant_id,name,created_at,updated_at) VALUES(1,1,'Synthetic alpha',1,1),(2,2,'Synthetic beta',1,1);
	INSERT INTO group_members(group_id,user_id) VALUES(1,2),(2,2);
	INSERT INTO api_keys(id,user_id,group_id,name,prefix,token_hash,created_at,encrypted_secret) VALUES
	 (1,2,1,'Synthetic alpha key','fake',zeroblob(32),1,x'01'),
	 (2,2,2,'Synthetic beta key','fake',randomblob(32),1,x'02');
	INSERT INTO usage_daily(day,user_id,group_id,provider,model,requests,input_tokens) VALUES(1,2,1,'codex','synthetic-model',1,42);
	INSERT INTO request_records(user_id,key_id,group_id,transport,operation,started_at,duration_ms,outcome) VALUES(2,1,1,'http','responses',1,1,'success');
	INSERT INTO allocation_schemes(id,name,group_id,enabled,created_at) VALUES(1,'Synthetic allowance',1,1,1);
	INSERT INTO allocation_revisions(id,scheme_id,effective_at,accounting_anchor_at,config) VALUES(1,1,1,1,'{}');
	INSERT INTO allocation_entries(request_id,scheme_id,revision_id,user_id,account_id,model,mode,window_start,reset_at,started_at,state)
	 VALUES('synthetic-pending',1,1,2,'synthetic-account','synthetic-model','tokens',1,100,1,'pending');`)
	if err != nil {
		t.Fatal(err)
	}
	return New(conn), conn
}

func TestRemoveMemberRevokesOnlyWorkspaceAccessAndPreservesHistory(t *testing.T) {
	s, conn := removalFixture(t)
	if _, err := conn.Exec("UPDATE memberships SET requests_per_minute=7,max_concurrency=3,first_request_at=9 WHERE tenant_id=1 AND user_id=2"); err != nil {
		t.Fatal(err)
	}
	ctx := audit.WithActor(context.Background(), audit.Actor{ID: 1, TenantID: 1, Username: "synthetic-owner", Role: "admin", Source: "user"})
	if err := s.RemoveMember(ctx, 1, 1, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("UPDATE memberships SET enabled=1 WHERE tenant_id=1 AND user_id=2"); err == nil {
		t.Fatal("removed membership re-enabled without explicit rejoin")
	}
	page, err := s.ListMembers(ctx, 1, 0)
	if err != nil || page.Total != 3 {
		t.Fatalf("removed member remains in roster: %+v %v", page, err)
	}
	if _, ok, err := s.Membership(ctx, 1, 2); err != nil || ok {
		t.Fatalf("removed member still has access: %v %v", ok, err)
	}
	if _, ok, err := s.Membership(ctx, 2, 2); err != nil || !ok {
		t.Fatalf("other workspace lost access: %v %v", ok, err)
	}
	checks := []struct {
		query string
		want  int
	}{
		{"SELECT count(*) FROM memberships WHERE tenant_id=1 AND user_id=2 AND removed_at IS NOT NULL AND enabled=0", 1},
		{"SELECT count(*) FROM users WHERE id=2 AND enabled=1", 1},
		{"SELECT count(*) FROM api_keys WHERE id=1 AND revoked_at IS NOT NULL AND encrypted_secret IS NULL", 1},
		{"SELECT count(*) FROM api_keys WHERE id=2 AND revoked_at IS NULL AND encrypted_secret IS NOT NULL", 1},
		{"SELECT count(*) FROM group_members WHERE user_id=2 AND group_id=1", 0},
		{"SELECT count(*) FROM group_members WHERE user_id=2 AND group_id=2", 1},
		{"SELECT input_tokens FROM usage_daily WHERE user_id=2 AND group_id=1", 42},
		{"SELECT count(*) FROM request_records WHERE user_id=2", 1},
		{"SELECT count(*) FROM allocation_entries WHERE user_id=2 AND state='pending'", 1},
		{"SELECT count(*) FROM audit_events WHERE tenant_id=1 AND action='member.remove' AND resource_id='2'", 1},
	}
	for _, check := range checks {
		var got int
		if err := conn.QueryRow(check.query).Scan(&got); err != nil || got != check.want {
			t.Fatalf("%s: got %d want %d err=%v", check.query, got, check.want, err)
		}
	}
	if _, err := s.SetMemberEnabled(ctx, 1, 1, 2, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("enable restored removed member: %v", err)
	}
	if _, err := s.SetMemberRole(ctx, 1, 1, 2, RoleMember); !errors.Is(err, ErrNotFound) {
		t.Fatalf("role update reached removed member: %v", err)
	}
	if ok, err := db.New(conn).MemberExistsInTenant(ctx, db.MemberExistsInTenantParams{TenantID: 1, UserID: 2}); err != nil || ok {
		t.Fatalf("removed member can receive grants: %v %v", ok, err)
	}
	if err := s.RemoveMember(ctx, 1, 1, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("repeat removal: %v", err)
	}
	member, err := s.AddMemberByUsername(ctx, 1, 1, "synthetic-member", RoleMember)
	if err != nil || !member.Enabled {
		t.Fatalf("explicit rejoin failed: %+v %v", member, err)
	}
	var rpm, concurrency, firstRequest int64
	if err := conn.QueryRow("SELECT requests_per_minute,max_concurrency,first_request_at FROM memberships WHERE tenant_id=1 AND user_id=2").Scan(&rpm, &concurrency, &firstRequest); err != nil || rpm != 0 || concurrency != 10 || firstRequest != 0 {
		t.Fatalf("rejoin retained old membership settings: rpm=%d concurrency=%d first_request=%d err=%v", rpm, concurrency, firstRequest, err)
	}
	var grants, revoked, used int
	if err := conn.QueryRow("SELECT count(*) FROM effective_group_access WHERE user_id=2 AND group_id=1").Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow("SELECT count(*) FROM api_keys WHERE id=1 AND revoked_at IS NOT NULL").Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow("SELECT input_tokens FROM usage_daily WHERE user_id=2 AND group_id=1").Scan(&used); err != nil {
		t.Fatal(err)
	}
	if grants != 0 || revoked != 1 || used != 42 {
		t.Fatalf("rejoin revived old access or reset usage: grants=%d revoked=%d used=%d", grants, revoked, used)
	}
}

func TestRemoveMemberEnforcesRoleAndWorkspaceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		actor, tenant, target int64
		want                  error
	}{
		{"member", 5, 1, 2, ErrForbidden}, {"owner", 4, 1, 1, ErrForbidden}, {"self", 4, 1, 4, ErrForbidden},
		{"other workspace", 1, 2, 2, ErrForbidden}, {"missing", 1, 1, 999, ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := removalFixture(t)
			if err := s.RemoveMember(context.Background(), tc.actor, tc.tenant, tc.target); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
	s, _ := removalFixture(t)
	if err := s.RemoveMember(context.Background(), 1, 1, 4); err != nil {
		t.Fatalf("owner cannot remove admin: %v", err)
	}
	s, conn := removalFixture(t)
	if _, err := conn.Exec("UPDATE memberships SET role='admin' WHERE tenant_id=1 AND user_id=5"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveMember(context.Background(), 4, 1, 5); !errors.Is(err, ErrForbidden) {
		t.Fatalf("admin removed peer: %v", err)
	}
	if _, err := conn.Exec("UPDATE memberships SET enabled=0 WHERE tenant_id=1 AND user_id=2"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveMember(context.Background(), 4, 1, 2); err != nil {
		t.Fatalf("admin cannot remove disabled member: %v", err)
	}
}

func TestRemoveMemberRollsBackWhenAuditFails(t *testing.T) {
	s, conn := removalFixture(t)
	if _, err := conn.Exec(`CREATE TRIGGER synthetic_audit_failure BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	ctx := audit.WithActor(context.Background(), audit.Actor{ID: 1, TenantID: 1, Username: "synthetic-owner", Role: "admin", Source: "user"})
	if err := s.RemoveMember(ctx, 1, 1, 2); err == nil {
		t.Fatal("audit failure ignored")
	}
	if _, ok, err := s.Membership(ctx, 1, 2); err != nil || !ok {
		t.Fatalf("failed removal changed access: %v %v", ok, err)
	}
	var keys, grants int
	if err := conn.QueryRow("SELECT count(*) FROM api_keys WHERE id=1 AND revoked_at IS NULL AND encrypted_secret IS NOT NULL").Scan(&keys); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow("SELECT count(*) FROM group_members WHERE group_id=1 AND user_id=2").Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if keys != 1 || grants != 1 {
		t.Fatalf("failed removal changed credentials: keys=%d grants=%d", keys, grants)
	}
}
