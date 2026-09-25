package storage

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
)

func TestTenantOwnedPoolRejectsForeignAccount(t *testing.T) {
	ctx := context.Background()
	connection, err := Open(ctx, filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	for _, id := range []int64{2, 3} {
		if _, err := connection.ExecContext(ctx, "INSERT INTO users(id,username,role,password_hash,enabled,created_at) VALUES(?,?, 'member','synthetic-hash',1,1)", id, "user-"+strconv.FormatInt(id, 10)); err != nil {
			t.Fatal(err)
		}
		if _, err := connection.ExecContext(ctx, "INSERT INTO tenants(id,name,owner_user_id,created_at) VALUES(?,?,?,1)", id, "Workspace", id); err != nil {
			t.Fatal(err)
		}
		if _, err := connection.ExecContext(ctx, "INSERT INTO memberships(tenant_id,user_id,role,created_at) VALUES(?,?,'owner',1)", id, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := connection.ExecContext(ctx, `INSERT INTO account_groups(id,name,created_at,updated_at,tenant_id)
		VALUES(20,'Pool A',1,1,2),(30,'Pool B',1,1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(ctx, `INSERT INTO accounts(id,provider,name,account_id,status,credential,created_at,updated_at,tenant_id)
		VALUES('account-a','codex','Account A','upstream-a','ready',x'01',1,1,2),
		('account-b','codex','Account B','upstream-b','ready',x'01',1,1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(ctx, `INSERT INTO account_groups(id,name,created_at,updated_at,tenant_id)
		VALUES(40,'Unassigned pool',1,1,3),(41,'Pool A',1,1,3)`); err != nil {
		t.Fatalf("pool name was globally unique instead of tenant-local: %v", err)
	}
	if _, err := connection.ExecContext(ctx, `INSERT INTO accounts(id,provider,name,account_id,status,credential,created_at,updated_at,tenant_id)
		VALUES('account-c','codex','Account C','upstream-a','ready',x'01',1,1,3)`); err != nil {
		t.Fatalf("upstream identity was globally unique instead of tenant-local: %v", err)
	}
	if _, err := connection.ExecContext(ctx, `INSERT INTO accounts(id,provider,name,account_id,status,credential,created_at,updated_at,tenant_id)
		VALUES('account-d','codex','Account D','unique-subject','ready',x'01',1,1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(ctx, "UPDATE account_groups SET tenant_id=2 WHERE id=40"); err == nil {
		t.Fatal("unassigned pool changed workspace ownership")
	}
	if _, err := connection.ExecContext(ctx, "UPDATE accounts SET tenant_id=2 WHERE id='account-d'"); err == nil {
		t.Fatal("unassigned account changed workspace ownership")
	}
	if _, err := connection.ExecContext(ctx, `INSERT INTO api_keys(user_id,group_id,name,prefix,token_hash,created_at)
		VALUES(2,20,'Synthetic key','sl_test',randomblob(32),1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(ctx, "UPDATE api_keys SET group_id=30 WHERE id=1"); err == nil {
		t.Fatal("existing key changed its workspace-bound pool")
	}
	if _, err := connection.ExecContext(ctx, "INSERT INTO group_members(group_id,user_id) VALUES(30,2)"); err != nil {
		t.Fatal(err)
	}
	var foreignAccess, ownerAccess bool
	if err := connection.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM effective_group_access
		WHERE group_id=30 AND user_id=2), EXISTS(SELECT 1 FROM effective_group_access
		WHERE group_id=30 AND user_id=3)`).Scan(&foreignAccess, &ownerAccess); err != nil || foreignAccess || !ownerAccess {
		t.Fatalf("pool authorization ignored workspace membership: foreign=%v owner=%v err=%v", foreignAccess, ownerAccess, err)
	}
	if _, err := connection.ExecContext(ctx, "INSERT INTO group_accounts(group_id,account_id) VALUES(20,'account-b')"); err == nil {
		t.Fatal("pool accepted an account from another workspace")
	}
	if _, err := connection.ExecContext(ctx, "INSERT INTO group_accounts(group_id,account_id) VALUES(20,'account-a')"); err != nil {
		t.Fatalf("same-tenant account was rejected: %v", err)
	}
	if _, err := connection.ExecContext(ctx, "UPDATE accounts SET tenant_id=3 WHERE id='account-a'"); err == nil {
		t.Fatal("account moved to another workspace while assigned to a pool")
	}
	if _, err := connection.ExecContext(ctx, "UPDATE account_groups SET tenant_id=3 WHERE id=20"); err == nil {
		t.Fatal("pool moved to another workspace while owning an account")
	}
}
