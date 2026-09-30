package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/content"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestBackupVerifiesContentRules(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	conn, err := storage.Open(ctx, filepath.Join(dir, databaseName))
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
	path := filepath.Join(dir, keyName)
	v, err := vault.Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	pattern := "SYNTHETIC_SECRET"
	if _, err = content.New(conn, v, 1).Update(ctx, content.Input{Mode: "block", Rules: []content.RuleInput{{Name: "Synthetic", Kind: "text", Pattern: &pattern, Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	if err = verifyCredentials(ctx, conn, dir); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if err = verifyCredentials(ctx, conn, dir); err == nil {
		t.Fatal("wrong content encryption key accepted")
	}
}
