package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestAccountProxyBindingAndSecrets(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	seedInitialTenant(t, connection)
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	service := New(connection, cipher)
	address := "http://synthetic-user:synthetic-secret@127.0.0.1:18080"
	proxy, err := service.CreateProxy(ctx, "Synthetic exit", address)
	if err != nil {
		t.Fatal(err)
	}
	if proxy.ID == "" || proxy.Endpoint != "http://127.0.0.1:18080" {
		t.Fatalf("proxy metadata: %+v", proxy)
	}
	list, err := service.ListProxies(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("proxy list: %+v %v", list, err)
	}
	public, _ := json.Marshal(list)
	if strings.Contains(string(public), "synthetic-secret") || strings.Contains(string(public), "synthetic-user") {
		t.Fatal("proxy credentials exposed")
	}
	var encrypted []byte
	if err := connection.QueryRow("SELECT address FROM proxies WHERE id=?", proxy.ID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encrypted), "synthetic-secret") {
		t.Fatal("plaintext proxy persisted")
	}
	account, err := service.Authorize(ctx, "Synthetic account", Credential{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", AccountID: "synthetic-account", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.BindProxy(ctx, account.ID, proxy.ID); err != nil {
		t.Fatal(err)
	}
	bound, err := service.Get(ctx, account.ID)
	if err != nil || bound.ProxyID != proxy.ID {
		t.Fatalf("account binding: %+v %v", bound, err)
	}
	credential, err := service.Prepare(ctx, account.ID, nil)
	if err != nil || credential.ProxyURL != address {
		t.Fatalf("prepared proxy: %q %v", credential.ProxyURL, err)
	}
	if err := service.DeleteProxy(ctx, proxy.ID); !errors.Is(err, ErrProxyInUse) {
		t.Fatalf("deleted bound proxy: %v", err)
	}
	next := "socks5://synthetic-user:next-secret@127.0.0.1:19090"
	if _, err := service.UpdateProxy(ctx, proxy.ID, "Updated exit", next); err != nil {
		t.Fatal(err)
	}
	credential, err = service.Prepare(ctx, account.ID, nil)
	if err != nil || credential.ProxyURL != next {
		t.Fatalf("updated proxy: %q %v", credential.ProxyURL, err)
	}
	if _, err := service.UpdateProxy(ctx, proxy.ID, "Renamed exit", ""); err != nil {
		t.Fatal(err)
	}
	credential, err = service.Prepare(ctx, account.ID, nil)
	if err != nil || credential.ProxyURL != next {
		t.Fatalf("renaming cleared proxy secret: %q %v", credential.ProxyURL, err)
	}
	if _, err := service.BindProxy(ctx, account.ID, ""); err != nil {
		t.Fatal(err)
	}
	credential, err = service.Prepare(ctx, account.ID, nil)
	if err != nil || credential.ProxyURL != "" {
		t.Fatalf("unbound proxy: %q %v", credential.ProxyURL, err)
	}
	if err := service.DeleteProxy(ctx, proxy.ID); err != nil {
		t.Fatal(err)
	}
}

func TestProxyRemainsBoundAcrossCredentialRefresh(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	seedInitialTenant(t, connection)
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	service := New(connection, cipher)
	address := "http://synthetic-user:synthetic-pass@127.0.0.1:18080"
	proxy, err := service.CreateProxy(ctx, "Synthetic exit", address)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.Authorize(ctx, "Synthetic account", Credential{AccessToken: "synthetic-old", RefreshToken: "synthetic-refresh", AccountID: "synthetic-account", ExpiresAt: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.BindProxy(ctx, account.ID, proxy.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := service.Prepare(ctx, account.ID, func(_ context.Context, old Credential) (Credential, error) {
		if old.ProxyURL != address {
			t.Fatalf("refresh missed proxy: %q", old.ProxyURL)
		}
		return Credential{AccessToken: "synthetic-new", RefreshToken: old.RefreshToken, AccountID: old.AccountID, ExpiresAt: time.Now().Add(time.Hour).Unix()}, nil
	})
	if err != nil || updated.ProxyURL != address {
		t.Fatalf("updated proxy: %q %v", updated.ProxyURL, err)
	}
	again, err := service.Prepare(ctx, account.ID, nil)
	if err != nil || again.ProxyURL != address {
		t.Fatalf("saved binding: %q %v", again.ProxyURL, err)
	}
	var encrypted []byte
	if err := connection.QueryRow("SELECT credential FROM accounts WHERE id=?", account.ID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	plaintext, err := cipher.Open(account.ID, encrypted)
	if err != nil || strings.Contains(string(plaintext), "synthetic-pass") {
		t.Fatal("proxy secret entered credential snapshot", err)
	}
}

func TestImportedAccountCanStartBoundToProxy(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	seedInitialTenant(t, connection)
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	service := New(connection, cipher)
	proxy, err := service.CreateProxy(ctx, "Synthetic exit", "http://127.0.0.1:18080")
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.ImportProviderWithProxy(ctx, "codex", "Synthetic account", []byte(`{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","account_id":"synthetic-account"}`), "", proxy.ID)
	if err != nil || account.ProxyID != proxy.ID {
		t.Fatalf("imported binding: %+v %v", account, err)
	}
	credential, err := service.Prepare(ctx, account.ID, nil)
	if !errors.Is(err, ErrRefresh) {
		t.Fatalf("expected expired synthetic credential to require refresh: %v, %+v", err, credential)
	}
	if err := service.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestProxyValidationAndWorkspaceIsolation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	seedInitialTenant(t, connection)
	if _, err := connection.Exec(`INSERT INTO users(id,username,role,password_hash,enabled,created_at) VALUES(2,'synthetic-second','member','synthetic-hash',1,1); INSERT INTO tenants(id,name,owner_user_id,created_at) VALUES(2,'Second workspace',2,1)`); err != nil {
		t.Fatal(err)
	}
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	first, second := New(connection, cipher), NewForTenant(connection, cipher, 2)
	for _, address := range []string{"file:///tmp/synthetic", "http://127.0.0.1", "http://127.0.0.1:0", "http://127.0.0.1:99999", "http://127.0.0.1:8080/path", "http://127.0.0.1:8080?secret=x", "http://127.0.0.1:8080#fragment", "http://user@127.0.0.1:8080"} {
		if _, err := first.CreateProxy(ctx, "Invalid", address); !errors.Is(err, ErrProxyInput) {
			t.Fatalf("accepted %q: %v", address, err)
		}
	}
	proxy, err := first.CreateProxy(ctx, "Owned exit", "http://127.0.0.1:18080")
	if err != nil {
		t.Fatal(err)
	}
	wrongKey, err := vault.Open(filepath.Join(dir, "wrong-key"), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := New(connection, wrongKey).Verify(ctx); !errors.Is(err, vault.ErrDecrypt) {
		t.Fatalf("proxy ciphertext survived key replacement: %v", err)
	}
	other, err := second.Authorize(ctx, "Other account", Credential{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", AccountID: "other-account", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.BindProxy(ctx, other.ID, proxy.ID); !errors.Is(err, ErrProxyNotFound) {
		t.Fatalf("cross-workspace binding: %v", err)
	}
	if _, err := second.UpdateProxy(ctx, proxy.ID, "Stolen", "http://127.0.0.1:18081"); !errors.Is(err, ErrProxyNotFound) {
		t.Fatalf("cross-workspace update: %v", err)
	}
	if err := second.DeleteProxy(ctx, proxy.ID); !errors.Is(err, ErrProxyNotFound) {
		t.Fatalf("cross-workspace delete: %v", err)
	}
}

func TestBulkProxyImportIsAtomic(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	seedInitialTenant(t, connection)
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	service := New(connection, cipher)
	batch := "http://198.51.100.10:80\nNamed exit | socks5://synthetic-user:synthetic-pass@127.0.0.1:19090\n127.0.0.1:18081"
	created, err := service.ImportProxies(ctx, batch)
	if err != nil || len(created) != 3 || created[0].Name != "198.51.100.10:80" || created[1].Name != "Named exit" || created[2].Endpoint != "http://127.0.0.1:18081" {
		t.Fatalf("bulk import: %+v %v", created, err)
	}
	if _, err := service.ImportProxies(ctx, "http://127.0.0.1:18082\nnot a proxy"); !errors.Is(err, ErrProxyInput) {
		t.Fatalf("accepted invalid batch: %v", err)
	}
	if _, err := service.ImportProxies(ctx, "Duplicate | http://127.0.0.1:18083\nNamed exit | http://127.0.0.1:18084"); !errors.Is(err, ErrProxyDuplicate) {
		t.Fatalf("accepted duplicate batch: %v", err)
	}
	list, err := service.ListProxies(ctx)
	if err != nil || len(list) != 3 {
		t.Fatalf("partial import: %+v %v", list, err)
	}
}

func TestProxyCheckResultRejectsStaleAddress(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	seedInitialTenant(t, connection)
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	service := New(connection, cipher)
	proxy, err := service.CreateProxy(ctx, "Synthetic exit", "http://127.0.0.1:18080")
	if err != nil {
		t.Fatal(err)
	}
	address, revision, err := service.ProxyCheckTarget(ctx, proxy.ID)
	if err != nil || address != "http://127.0.0.1:18080" {
		t.Fatalf("target: %q %d %v", address, revision, err)
	}
	observation := ProxyObservation{Reachable: true, ExitIP: "203.0.113.8", Country: "US", Region: "Test region", City: "Test city", LatencyMS: 42}
	checked, err := service.SaveProxyCheck(ctx, proxy.ID, revision, observation)
	if err != nil || !checked.Reachable || checked.ExitIP != observation.ExitIP || checked.Country != "US" || checked.CheckedAt == 0 {
		t.Fatalf("check: %+v %v", checked, err)
	}
	if _, err := service.UpdateProxy(ctx, proxy.ID, proxy.Name, "http://127.0.0.1:18081"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveProxyCheck(ctx, proxy.ID, revision, observation); !errors.Is(err, ErrProxyChanged) {
		t.Fatalf("stale result published: %v", err)
	}
	list, err := service.ListProxies(ctx)
	if err != nil || list[0].CheckedAt != 0 || list[0].ExitIP != "" {
		t.Fatalf("old location retained: %+v %v", list, err)
	}
}

func TestDeleteFailedProxiesKeepsBoundAndStaleEntries(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	seedInitialTenant(t, connection)
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	service := New(connection, cipher)
	now := time.Now().Truncate(time.Second)
	service.now = func() time.Time { return now }
	ids := make(map[string]string)
	for index, name := range []string{"Failed", "Bound", "Lookup", "Stale"} {
		proxy, err := service.CreateProxy(ctx, name, "http://127.0.0.1:"+strconv.Itoa(18080+index))
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = proxy.ID
	}
	account, err := service.Authorize(ctx, "Synthetic account", Credential{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", AccountID: "synthetic-account", ExpiresAt: now.Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.BindProxy(ctx, account.ID, ids["Bound"]); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Failed", "Bound", "Lookup", "Stale"} {
		if name == "Stale" {
			service.now = func() time.Time { return now.Add(-31 * time.Minute) }
		}
		_, revision, err := service.ProxyCheckTarget(ctx, ids[name])
		if err != nil {
			t.Fatal(err)
		}
		code := "connection_failed"
		if name == "Lookup" {
			code = "lookup_rate_limited"
		}
		if _, err := service.SaveProxyCheck(ctx, ids[name], revision, ProxyObservation{ErrorCode: code}); err != nil {
			t.Fatal(err)
		}
		service.now = func() time.Time { return now }
	}
	before, err := service.ListProxies(ctx)
	if err != nil || len(before) != 4 {
		t.Fatalf("proxy eligibility: %+v %v", before, err)
	}
	for _, row := range before {
		if row.PruneEligible != (row.ID == ids["Failed"]) {
			t.Fatalf("unexpected prune eligibility: %+v", row)
		}
	}
	deleted, err := service.DeleteFailedProxies(ctx)
	if err != nil || deleted != 1 {
		t.Fatalf("deleted %d: %v", deleted, err)
	}
	rows, err := service.ListProxies(ctx)
	if err != nil || len(rows) != 3 {
		t.Fatalf("remaining proxies: %+v %v", rows, err)
	}
	for _, row := range rows {
		if row.ID == ids["Failed"] {
			t.Fatal("failed unbound proxy remained")
		}
	}
}
