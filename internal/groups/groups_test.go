package groups_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
)

func fixture(t *testing.T) (*sql.DB, *groups.Service, auth.Member, accounts.Account) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "groups.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	identity, err := auth.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "synthetic-admin", "synthetic-pass"); err != nil {
		t.Fatal(err)
	}
	member, err := identity.CreateMember(ctx, "synthetic-member", "synthetic-pass")
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	account, err := accounts.New(connection, cipher).Authorize(ctx, "Synthetic", accounts.Credential{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", AccountID: "synthetic-subject", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	return connection, groups.New(connection), member, account
}

func TestNewAccountsAndMembersRemainUnassigned(t *testing.T) {
	connection, service, member, _ := fixture(t)
	ctx := context.Background()
	choices, err := service.Available(ctx, member.ID)
	if err != nil || len(choices) != 0 {
		t.Fatalf("unexpected automatic access: %+v, %v", choices, err)
	}
	for _, table := range []string{"group_accounts", "group_members"} {
		var count int
		if err := connection.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("automatic membership in %s: %d, %v", table, count, err)
		}
	}
}

func TestFirstPoolCanBeRenamedAndDisabled(t *testing.T) {
	_, service, _, account := fixture(t)
	ctx := context.Background()
	pools, err := service.List(ctx)
	if err != nil || len(pools) != 0 {
		t.Fatalf("fresh instance created a pool: %+v, %v", pools, err)
	}
	pool, err := service.Save(ctx, 0, groups.Input{Name: "Synthetic first pool", Enabled: true, AccountIDs: []string{account.ID}})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.Save(ctx, pool.ID, groups.Input{Name: "Synthetic renamed", Enabled: false, AccountIDs: []string{account.ID}})
	if err != nil || updated.Name != "Synthetic renamed" || updated.Enabled || updated.IsDefault {
		t.Fatalf("first pool still special: %+v, %v", updated, err)
	}
}

func TestPoolAndMemberChangesAreAtomicAndScoped(t *testing.T) {
	_, service, member, account := assignedFixture(t)
	ctx := context.Background()
	pool, err := service.Save(ctx, 0, groups.Input{Name: "Project alpha", Enabled: true, AccountIDs: []string{account.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Save(ctx, 0, groups.Input{Name: "project alpha", Enabled: true, AccountIDs: []string{}}); !errors.Is(err, groups.ErrDuplicate) {
		t.Fatal("duplicate pool accepted", err)
	}
	if _, err := service.Save(ctx, pool.ID, groups.Input{Name: "Changed", Enabled: true, AccountIDs: []string{"missing-account"}}); !errors.Is(err, groups.ErrInput) {
		t.Fatal("missing account accepted", err)
	}
	value, err := service.Get(ctx, pool.ID)
	if err != nil || value.Name != "Project alpha" || len(value.AccountIDs) != 1 {
		t.Fatal("failed update changed pool", err)
	}
	if err := service.SetMemberGroups(ctx, member.ID, []int64{pool.ID}); err != nil {
		t.Fatal(err)
	}
	choices, err := service.Available(ctx, member.ID)
	if err != nil || len(choices) != 1 || choices[0].ID != pool.ID {
		t.Fatal("grant does not constrain choices", err)
	}
	if err := service.SetMemberGroups(ctx, member.ID, []int64{999}); !errors.Is(err, groups.ErrInput) {
		t.Fatal("missing group accepted", err)
	}
	ids, err := service.MemberGroups(ctx, member.ID)
	if err != nil || len(ids) != 1 || ids[0] != pool.ID {
		t.Fatal("failed grant update removed access", err)
	}
	if _, err := service.Save(ctx, pool.ID, groups.Input{Name: pool.Name, Enabled: false, AccountIDs: []string{account.ID}}); err != nil {
		t.Fatal(err)
	}
	choices, err = service.Available(ctx, member.ID)
	if err != nil || len(choices) != 0 {
		t.Fatal("disabled pool visible", err)
	}
	if err := service.SetMemberGroups(ctx, member.ID, []int64{}); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionReadinessOnlyUsesAuthorizedGroups(t *testing.T) {
	_, service, member, _ := assignedFixture(t)
	ctx := context.Background()
	status, err := service.Connection(ctx, member.ID)
	if err != nil || status != "ready" {
		t.Fatal(status, err)
	}
	empty, err := service.Save(ctx, 0, groups.Input{Name: "Empty", Enabled: true, AccountIDs: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetMemberGroups(ctx, member.ID, []int64{empty.ID}); err != nil {
		t.Fatal(err)
	}
	status, err = service.Connection(ctx, member.ID)
	if err != nil || status != "not_configured" {
		t.Fatal("another pool leaked readiness", status, err)
	}
}

func TestMembersRequireTeamAccessInsteadOfDirectGrants(t *testing.T) {
	connection, service, member, _ := assignedFixture(t)
	ctx := context.Background()
	if _, err := connection.Exec("INSERT INTO allocation_teams(name,enabled,created_at) VALUES('Synthetic configured team',1,1)"); err != nil {
		t.Fatal(err)
	}
	// The legacy default grant exists but must no longer authorize a member.
	choices, err := service.Available(ctx, member.ID)
	if err != nil || len(choices) != 0 {
		t.Fatalf("member without a team saw groups: %+v, %v", choices, err)
	}
	if _, err := groups.ReadPolicy(ctx, db.New(connection), member.ID, groups.LegacyID); !errors.Is(err, groups.ErrUnavailable) {
		t.Fatalf("default grant bypassed team policy: %v", err)
	}
	status, err := service.Connection(ctx, member.ID)
	if err != nil || status != "not_configured" {
		t.Fatalf("unauthorized account leaked readiness: %s, %v", status, err)
	}
	choices, err = service.Available(ctx, 1)
	if err != nil || len(choices) != 1 {
		t.Fatalf("administrator lost management access: %+v, %v", choices, err)
	}
}

func assignedFixture(t *testing.T) (*sql.DB, *groups.Service, auth.Member, accounts.Account) {
	conn, service, member, account := fixture(t)
	pool, err := service.Save(context.Background(), 0, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{account.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetMemberGroups(context.Background(), member.ID, []int64{pool.ID}); err != nil {
		t.Fatal(err)
	}
	return conn, service, member, account
}
