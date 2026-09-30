package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/content"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestContentOnlyInstanceRequiresOriginalVaultKey(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	conn, err := storage.Open(ctx, filepath.Join(dir, "sublane.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	identity, err := auth.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = identity.Setup(ctx, "synthetic-owner", "synthetic-pass", "Synthetic"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials.key")
	v, err := vault.Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	pattern := "SYNTHETIC_SECRET"
	if _, err = content.New(conn, v, 1).Update(ctx, content.Input{Mode: "block", Rules: []content.RuleInput{{Name: "Synthetic", Kind: "text", Pattern: &pattern, Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err = openVault(ctx, conn, dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing content key recreated", err)
	}
}
