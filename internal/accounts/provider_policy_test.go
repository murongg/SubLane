package accounts

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestCodexOnlyAccountPolicyPreservesButBlocksOtherProviders(t *testing.T) {
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
	legacy := New(connection, cipher)
	credential := Credential{Provider: "claude", AccountID: "synthetic-legacy", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	old, err := legacy.Authorize(ctx, "Synthetic legacy", credential, "")
	if err != nil {
		t.Fatal(err)
	}
	service := New(connection, cipher)
	service.RestrictToCodex()
	if !service.ProviderEnabled("codex") || service.ProviderEnabled("claude") || service.ProviderEnabled("antigravity") {
		t.Fatal("incorrect active providers")
	}
	if _, err := service.Authorize(ctx, "Synthetic other", credential, ""); !errors.Is(err, ErrProviderDisabled) {
		t.Fatal("non-Codex authorization accepted", err)
	}
	if _, err := service.ImportProvider(ctx, "claude", "Synthetic other", []byte(`{}`), ""); !errors.Is(err, ErrProviderDisabled) {
		t.Fatal("non-Codex import accepted", err)
	}
	if _, err := service.Prepare(ctx, old.ID, nil); !errors.Is(err, ErrProviderDisabled) {
		t.Fatal("stored non-Codex credential used", err)
	}
	if _, err := service.SetEnabled(ctx, old.ID, true); !errors.Is(err, ErrProviderDisabled) {
		t.Fatal("stored non-Codex account enabled", err)
	}
	if rows, err := service.List(ctx); err != nil || len(rows) != 1 || rows[0].ID != old.ID {
		t.Fatal("stored account was lost", rows, err)
	}
	codex := Credential{AccountID: "synthetic-codex", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	if _, err := service.Authorize(ctx, "Synthetic Codex", codex, ""); err != nil {
		t.Fatal("Codex authorization blocked", err)
	}
}
