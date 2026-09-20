package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestMaintenanceBackupVerifyAndRestore(t *testing.T) {
	ctx := context.Background()
	source := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(source, "sublane.db"))
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
	if _, err := vault.Open(filepath.Join(source, "credentials.key"), true); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "test.tar.gz")
	target := filepath.Join(t.TempDir(), "restored")
	for _, args := range [][]string{{"backup", "--output", archive}, {"backup", "verify", "--input", archive}, {"restore", "--input", archive, "--data-dir", target}} {
		var output bytes.Buffer
		if err := runMaintenance(ctx, args, source, &output); err != nil {
			t.Fatal(args[0], err)
		}
		if output.Len() == 0 {
			t.Fatal("missing command result")
		}
	}
	if _, err := os.Stat(filepath.Join(target, "sublane.db")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"backup"}, {"backup", "verify"}, {"restore", "--input", archive}, {"backup", "--input", archive}, {"restore", "--input", archive, "--data-dir", source}, {"backup", "--output", archive, "unexpected"}} {
		if err := runMaintenance(ctx, args, source, &bytes.Buffer{}); err == nil {
			t.Fatal("unsafe or invalid arguments accepted", args)
		}
	}
	if err := runMaintenance(ctx, []string{"backup", "--help"}, source, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}
