package tenants

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/murongg/SubLane/internal/storage"
)

func TestMembershipIsScopedToTenant(t *testing.T) {
	ctx := context.Background()
	connection, err := storage.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	for _, id := range []int64{2, 3} {
		_, err = connection.ExecContext(ctx,
			"INSERT INTO users(id,username,role,password_hash,enabled,created_at) VALUES(?,?,'member','synthetic-hash',1,1)",
			id, "member-"+strconv.FormatInt(id, 10))
		if err != nil {
			t.Fatal(err)
		}
	}
	service := New(connection)
	if _, err := service.Create(ctx, 2, "Bad\nname"); err != ErrInput {
		t.Fatalf("control character accepted in workspace name: %v", err)
	}
	if _, err := service.Create(ctx, 3, "这是一个长度合理的中文工作空间名称示例示例示例"); err != nil {
		t.Fatalf("valid non-ASCII workspace name rejected: %v", err)
	}
	alpha, err := service.Create(ctx, 2, "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	beta, err := service.Create(ctx, 3, "Beta")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AddMember(ctx, 2, alpha.ID, 3, RoleMember); err != nil {
		t.Fatal(err)
	}
	if err := service.AddMember(ctx, 2, alpha.ID, 2, RoleMember); err != ErrForbidden {
		t.Fatalf("owner membership was changed through AddMember: %v", err)
	}
	if err := service.AddMember(ctx, 2, beta.ID, 2, RoleMember); err != ErrForbidden {
		t.Fatalf("non-member actor added a member across tenants: %v", err)
	}
	listed, err := service.List(ctx, 2)
	if err != nil || len(listed) != 1 || listed[0].ID != alpha.ID {
		t.Fatalf("workspace list leaked another tenant: %+v, %v", listed, err)
	}

	checks := []struct {
		tenant int64
		user   int64
		role   Role
		ok     bool
	}{
		{alpha.ID, 2, RoleOwner, true},
		{alpha.ID, 3, RoleMember, true},
		{beta.ID, 3, RoleOwner, true},
		{beta.ID, 2, "", false},
	}
	for _, check := range checks {
		member, ok, err := service.Membership(ctx, check.tenant, check.user)
		if err != nil {
			t.Fatal(err)
		}
		if ok != check.ok || member.Role != check.role {
			t.Fatalf("membership tenant=%d user=%d: got (%+v, %v), want (%q, %v)", check.tenant, check.user, member, ok, check.role, check.ok)
		}
	}
	if err := service.Suspend(ctx, alpha.ID); err != nil {
		t.Fatal(err)
	}
	_, ok, err := service.Membership(ctx, alpha.ID, 3)
	if err != nil || ok {
		t.Fatalf("suspended tenant should deny membership: ok=%v err=%v", ok, err)
	}
	_, ok, err = service.Membership(ctx, beta.ID, 3)
	if err != nil || !ok {
		t.Fatalf("suspending another tenant affected beta: ok=%v err=%v", ok, err)
	}
}
