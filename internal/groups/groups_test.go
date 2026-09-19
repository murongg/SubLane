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

func TestNewMembersAndAccountsJoinDefaultGroup(t *testing.T) {
	_, service, member, account := fixture(t)
	ctx := context.Background()
	choices, err := service.Available(ctx, member.ID)
	if err != nil || len(choices) != 1 || choices[0].ID != groups.DefaultID {
		t.Fatal("new member lost default access", err)
	}
	value, err := service.Get(ctx, groups.DefaultID)
	if err != nil || len(value.AccountIDs) != 1 || value.AccountIDs[0] != account.ID {
		t.Fatal("new account missing from default pool", err)
	}
	if _, err := service.Save(ctx, groups.DefaultID, groups.Input{Name: "Default", Enabled: false, AccountIDs: []string{account.ID}}); !errors.Is(err, groups.ErrDefault) {
		t.Fatal("default pool disabled", err)
	}
}

func TestPoolAndMemberChangesAreAtomicAndScoped(t *testing.T) {
	_, service, member, account := fixture(t)
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
	_, service, member, _ := fixture(t)
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
