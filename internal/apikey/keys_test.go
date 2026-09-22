package apikey

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage"
)

func TestKeyOwnershipHashingAndRevocation(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "keys.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	identity, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "owner-test", "owner pass 42"); err != nil {
		t.Fatal(err)
	}
	member, err := identity.CreateMember(ctx, "member-test", "member pass 42")
	if err != nil {
		t.Fatal(err)
	}
	keys := newTestKeys(t, db)
	created, err := keys.CreateInGroup(ctx, member.ID, 1, "Laptop test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Secret, "sl_") || len(created.Secret) != 46 {
		t.Fatal("invalid key format")
	}
	var digest []byte
	if err := db.QueryRow("SELECT token_hash FROM api_keys WHERE id = ?", created.Key.ID).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256([]byte(created.Secret))
	if string(digest) != string(expected[:]) {
		t.Fatal("plaintext key is not hashed")
	}
	page, err := keys.List(ctx, member.ID, 0)
	if err != nil || len(page.Keys) != 1 {
		t.Fatal("missing own key", err)
	}
	payload, _ := json.Marshal(page)
	if strings.Contains(string(payload), created.Secret) || strings.Contains(string(payload), "token_hash") {
		t.Fatal("secret leaked in metadata")
	}
	other, err := keys.List(ctx, 1, 0)
	if err != nil || len(other.Keys) != 0 {
		t.Fatal("another user's keys were exposed")
	}
	if _, err := keys.Revoke(ctx, 1, created.Key.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-user revocation accepted")
	}
	principal, err := keys.Authenticate(ctx, created.Secret)
	if err != nil || principal.UserID != member.ID {
		t.Fatal("key authentication failed", err)
	}
	if _, err := identity.SetMemberEnabled(ctx, member.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Authenticate(ctx, created.Secret); !errors.Is(err, ErrInvalidKey) {
		t.Fatal("disabled member key worked")
	}
	if _, err := identity.SetMemberEnabled(ctx, member.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Authenticate(ctx, created.Secret); err != nil {
		t.Fatal("active key not restored after re-enable", err)
	}
	if _, err := keys.Revoke(ctx, member.ID, created.Key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Authenticate(ctx, created.Secret); !errors.Is(err, ErrInvalidKey) {
		t.Fatal("revoked key worked")
	}
	page, err = keys.List(ctx, member.ID, 0)
	if err != nil || page.Keys[0].RevokedAt == nil || page.Keys[0].LastUsedAt == nil {
		t.Fatal("missing key lifecycle metadata", err)
	}
}

func TestKeyLimitAndInvalidInput(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "limit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	identity, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "owner-test", "owner pass 42"); err != nil {
		t.Fatal(err)
	}
	keys := newTestKeys(t, db)
	for _, name := range []string{"", "   ", strings.Repeat("x", 65)} {
		if _, err := keys.CreateInGroup(ctx, 1, 1, name); !errors.Is(err, ErrInput) {
			t.Fatal("invalid key name accepted")
		}
	}
	var first int64
	for i := 0; i < 20; i++ {
		key, err := keys.CreateInGroup(ctx, 1, 1, "Synthetic key")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = key.Key.ID
		}
	}
	if _, err := keys.CreateInGroup(ctx, 1, 1, "Too many"); !errors.Is(err, ErrLimit) {
		t.Fatal("active key limit not enforced")
	}
	if _, err := keys.Revoke(ctx, 1, first); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.CreateInGroup(ctx, 1, 1, "Replacement"); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "sl_short", strings.Repeat("x", 46)} {
		if _, err := keys.Authenticate(ctx, token); !errors.Is(err, ErrInvalidKey) {
			t.Fatal("invalid token accepted")
		}
	}
}

func TestKeysEnforceGroupGrantsOnCreationAndEveryAuthentication(t *testing.T) {
	ctx := context.Background()
	connection, err := storage.Open(ctx, filepath.Join(t.TempDir(), "groups.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
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
	configureTestPool(t, connection)
	pools := groups.New(connection)
	pool, err := pools.Save(ctx, 0, groups.Input{Name: "Private pool", Enabled: true, AccountIDs: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	keys := newTestKeys(t, connection)
	if _, err := keys.CreateInGroup(ctx, member.ID, pool.ID, "Denied"); !errors.Is(err, groups.ErrUnavailable) {
		t.Fatal("unauthorized group accepted", err)
	}
	if err := pools.SetMemberGroups(ctx, member.ID, []int64{pool.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.CreateInGroup(ctx, member.ID, 1, "Default denied"); !errors.Is(err, groups.ErrUnavailable) {
		t.Fatal("omitted group bypassed revoked default grant", err)
	}
	created, err := keys.CreateInGroup(ctx, member.ID, pool.ID, "Synthetic key")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := keys.Authenticate(ctx, created.Secret)
	if err != nil || principal.GroupID != pool.ID {
		t.Fatal("key group not preserved", err)
	}
	if err := pools.SetMemberGroups(ctx, member.ID, []int64{}); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Authenticate(ctx, created.Secret); !errors.Is(err, ErrInvalidKey) {
		t.Fatal("revoked group access remained usable", err)
	}
	page, err := keys.List(ctx, member.ID, 0)
	if err != nil || page.Keys[0].GroupAccess != "blocked" {
		t.Fatal("blocked group not reflected in metadata", err)
	}
	if err := pools.SetMemberGroups(ctx, member.ID, []int64{pool.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := pools.Save(ctx, pool.ID, groups.Input{Name: pool.Name, Enabled: false, AccountIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Authenticate(ctx, created.Secret); !errors.Is(err, ErrInvalidKey) {
		t.Fatal("disabled group key worked", err)
	}
}

func TestManagedPoolRequiresSchemeBinding(t *testing.T) {
	ctx := context.Background()
	conn, err := storage.Open(ctx, filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	identity, err := auth.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = identity.Setup(ctx, "synthetic-admin", "synthetic-pass"); err != nil {
		t.Fatal(err)
	}
	member, err := identity.CreateMember(ctx, "synthetic-user", "synthetic-pass")
	if err != nil {
		t.Fatal(err)
	}
	keys := newTestKeys(t, conn)
	legacy, err := keys.CreateInGroup(ctx, member.ID, 1, "Legacy")
	if err != nil {
		t.Fatal(err)
	}
	// Minimal synthetic scheme fixture deliberately exercises authentication independent of management validation.
	for _, stmt := range []string{"INSERT INTO allocation_teams(id,name,enabled,created_at) VALUES(1,'Synthetic',1,1)", "INSERT INTO allocation_team_members(team_id,user_id) VALUES(1,2)", "INSERT INTO allocation_schemes(id,name,team_id,group_id,enabled,created_at) VALUES(1,'Synthetic',1,1,1,1)", `INSERT INTO allocation_revisions(scheme_id,effective_at,config) VALUES(1,1,'{"mode":"tokens","period":"day","members":[{"user_id":2,"limit":100}],"rates":[] }')`} {
		if _, err = conn.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = keys.Authenticate(ctx, legacy.Secret); !errors.Is(err, ErrInvalidKey) {
		t.Fatal("legacy key bypassed managed pool", err)
	}
	bound, err := keys.CreateInScheme(ctx, member.ID, 1, "Bound", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = keys.Authenticate(ctx, bound.Secret); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec("DELETE FROM allocation_team_members WHERE user_id=2"); err != nil {
		t.Fatal(err)
	}
	if _, err = keys.Authenticate(ctx, bound.Secret); !errors.Is(err, ErrInvalidKey) {
		t.Fatal("removed member retained access", err)
	}
}
