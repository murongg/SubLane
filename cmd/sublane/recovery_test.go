package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
)

func TestAdministratorRecoveryUsesExistingDatabaseAndBoundedStdin(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := recoverPassword(ctx, dir, strings.NewReader("synthetic-new-pass")); err == nil {
		t.Fatal("recovery created missing database")
	}
	if _, err := os.Stat(filepath.Join(dir, "sublane.db")); !os.IsNotExist(err) {
		t.Fatal("missing database was created")
	}
	connection, err := storage.Open(ctx, filepath.Join(dir, "sublane.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	identity, err := auth.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	session, err := identity.Setup(ctx, "owner-test", "synthetic-old-pass", "Synthetic workspace")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"short", strings.Repeat("x", 83), "new-password\nextra"} {
		if err := recoverPassword(ctx, dir, strings.NewReader(input)); err == nil {
			t.Fatal("invalid stdin accepted")
		}
	}
	if err := recoverPassword(ctx, dir, strings.NewReader("synthetic-new-pass\r\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Login(ctx, "owner-test", "synthetic-new-pass"); err != nil {
		t.Fatal(err)
	}
	state, err := identity.State(ctx, session.Token)
	if err != nil || state.User != nil {
		t.Fatal("recovery did not revoke sessions", err)
	}
}
