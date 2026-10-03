package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestAPIKeyAccountLifecycle(t *testing.T) {
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
	raw := []byte(`{"api_key":"synthetic-api-key","base_url":"https://gateway.example.test/v1/"}`)
	account, err := service.ImportProvider(ctx, "openai", "Synthetic API", raw, "")
	if err != nil {
		t.Fatalf("API key account import: %v", err)
	}
	if account.ExpiresAt != 0 || account.Status != "unverified" {
		t.Fatal("static key lifecycle", account)
	}
	encoded, _ := json.Marshal(account)
	if strings.Contains(string(encoded), "synthetic-api-key") {
		t.Fatal("API key disclosed in metadata")
	}
	if !strings.Contains(string(encoded), "https://gateway.example.test/v1") {
		t.Fatal("endpoint missing from metadata")
	}
	var stored []byte
	if err := connection.QueryRow(`SELECT credential FROM accounts WHERE id=?`, account.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "synthetic-api-key") {
		t.Fatal("key persisted without encryption")
	}
	credential, err := service.Prepare(ctx, account.ID, func(context.Context, Credential) (Credential, error) {
		t.Fatal("static key entered OAuth refresh")
		return Credential{}, nil
	})
	if err != nil || credential.AccessToken != "synthetic-api-key" {
		t.Fatal("static key preparation", err)
	}
	paused, err := service.SetEnabled(ctx, account.ID, false)
	if err != nil || paused.BaseURL != "https://gateway.example.test/v1" {
		t.Fatal("endpoint lost after account update", paused, err)
	}
	if _, err := service.SetEnabled(ctx, account.ID, true); err != nil {
		t.Fatal(err)
	}
	bound, err := service.BindProxy(ctx, account.ID, "")
	if err != nil || bound.BaseURL != "https://gateway.example.test/v1" {
		t.Fatal("endpoint lost after proxy update", bound, err)
	}
	if _, err := service.RefreshAfterRejection(ctx, account.ID, credential.AccessToken, nil); !errors.Is(err, ErrReauthorize) {
		t.Fatal("rejected API key must require replacement", err)
	}
	if _, err := service.Prepare(ctx, account.ID, nil); !errors.Is(err, ErrReauthorize) {
		t.Fatal("rejected key eligible again", err)
	}
	rotated := []byte(`{"api_key":"synthetic-rotated-key","base_url":"https://gateway.example.test/v1"}`)
	updated, err := service.ImportProvider(ctx, "openai", account.Name, rotated, account.ID)
	if err != nil || updated.ID != account.ID {
		t.Fatal("key rotation changed account identity", err)
	}
	credential, err = service.Prepare(ctx, account.ID, nil)
	if err != nil || credential.AccessToken != "synthetic-rotated-key" {
		t.Fatal("rotated key not usable", err)
	}
	changed := []byte(`{"api_key":"synthetic-rotated-key","base_url":"https://other.example.test/v1"}`)
	if _, err := service.ImportProvider(ctx, "openai", account.Name, changed, account.ID); !errors.Is(err, ErrIdentity) {
		t.Fatal("endpoint rebind changed existing conversation identity", err)
	}
	if _, err := service.ImportProvider(ctx, "openai", "Duplicate", rotated, ""); !errors.Is(err, ErrDuplicate) {
		t.Fatal("duplicate rotated key accepted", err)
	}
	if _, err := service.ImportProvider(ctx, "openai", "Separate key", raw, ""); err != nil {
		t.Fatal("rotation reserved a key no longer stored on the original account", err)
	}
	if err := service.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAPIKeyCredentialValidation(t *testing.T) {
	for _, endpoint := range []string{"https://api.openai.com/v1", "https://relay.example.test/api/v1/", "http://127.0.0.1:1234/v1"} {
		raw, _ := json.Marshal(map[string]string{"api_key": "synthetic-key", "base_url": endpoint})
		if _, err := ParseFor("openai", raw); err != nil {
			t.Fatalf("valid endpoint %q: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{"", "ftp://example.test/v1", "https://user:secret@example.test/v1", "https://example.test/v1?key=secret", "https://example.test/v1#fragment", "/v1", "https://", "https://example.test/%2e%2e/v1"} {
		raw, _ := json.Marshal(map[string]string{"api_key": "synthetic-key", "base_url": endpoint})
		if _, err := ParseFor("openai", raw); !errors.Is(err, ErrInput) {
			t.Fatalf("unsafe endpoint %q accepted: %v", endpoint, err)
		}
	}
	for _, key := range []string{"", "synthetic\nkey", strings.Repeat("x", 16385)} {
		raw, _ := json.Marshal(map[string]string{"api_key": key, "base_url": "https://example.test/v1"})
		if _, err := ParseFor("openai", raw); !errors.Is(err, ErrInput) {
			t.Fatal("invalid key accepted", err)
		}
	}
}
