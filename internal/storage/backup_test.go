package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotIncludesCommittedWALWithoutChangingSource(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	connection, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := connection.Exec("PRAGMA wal_autocheckpoint=0; INSERT INTO settings(key,value) VALUES('synthetic.backup','before')"); err != nil {
		t.Fatal(err)
	}
	wal, err := os.Stat(source + "-wal")
	if err != nil || wal.Size() == 0 {
		t.Fatal("fixture has no WAL", err)
	}
	target := filepath.Join(t.TempDir(), "snapshot.db")
	if err := Snapshot(ctx, source, target, 4<<20); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec("UPDATE settings SET value='after' WHERE key='synthetic.backup'"); err != nil {
		t.Fatal(err)
	}
	restored, err := OpenReadOnly(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var value string
	if err := restored.QueryRow("SELECT value FROM settings WHERE key='synthetic.backup'").Scan(&value); err != nil || value != "before" {
		t.Fatal("WAL snapshot not isolated", value, err)
	}
	if _, err := restored.Exec("DELETE FROM settings"); err == nil {
		t.Fatal("read-only database accepted writes")
	}
	schema, err := ValidateSnapshot(ctx, restored)
	if err != nil || schema != 19 {
		t.Fatal("schema validation", schema, err)
	}
	if err := Snapshot(ctx, source, target, 4<<20); err == nil {
		t.Fatal("snapshot overwrote existing file")
	}
	if err := restored.QueryRow("SELECT value FROM settings WHERE key='synthetic.backup'").Scan(&value); err != nil || value != "before" {
		t.Fatal("existing snapshot changed", value, err)
	}
}
func TestSnapshotMissingSourceCancellationAndBounds(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	if _, err := OpenReadOnly(ctx, source); err == nil {
		t.Fatal("opened missing source")
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatal("created missing source", err)
	}
	connection, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	for _, test := range []struct {
		ctx   context.Context
		limit int64
		name  string
	}{{canceled, 4 << 20, "canceled"}, {ctx, 1, "oversized"}} {
		target := filepath.Join(dir, test.name+".db")
		err := Snapshot(test.ctx, source, target, test.limit)
		if err == nil {
			t.Fatal("invalid snapshot accepted", test.name)
		}
		if test.name == "canceled" && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatal("partial snapshot leaked", err)
		}
	}
	if _, err := connection.Exec("INSERT INTO schema_migrations(name) VALUES('999_future.sql')"); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSnapshot(ctx, connection); err == nil {
		t.Fatal("future schema accepted")
	}
}
