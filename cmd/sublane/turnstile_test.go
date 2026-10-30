package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/turnstile"
	"github.com/murongg/SubLane/internal/vault"
)

func TestTurnstilePreventsMissingVaultKeyReplacement(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	conn, err := storage.Open(ctx, filepath.Join(dir, "sublane.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	path := filepath.Join(dir, "credentials.key")
	v, _ := vault.Open(path, true)
	s, err := turnstile.New(ctx, conn, v, nil)
	if err != nil {
		t.Fatal(err)
	}
	secret := "synthetic-secret"
	if _, err := s.Save(ctx, turnstile.Input{Enabled: true, SiteKey: "synthetic-site", SecretKey: &secret}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := openVault(ctx, conn, dir, ""); err == nil {
		t.Fatal("replaced missing key despite encrypted Turnstile configuration")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("created replacement key", err)
	}
}
