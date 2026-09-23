package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenInitializesSchemaAndPreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "synthetic # database.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var journal string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil || journal != "wal" {
		t.Fatalf("journal=%s err=%v", journal, err)
	}
	var foreignKeys int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatalf("foreign_keys=%d err=%v", foreignKeys, err)
	}
	if _, err := db.Exec("INSERT INTO settings (key, value) VALUES (?, ?)", "synthetic-setting", "synthetic-value"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var value string
	if err := db.QueryRow("SELECT value FROM settings WHERE key = ?", "synthetic-setting").Scan(&value); err != nil || value != "synthetic-value" {
		t.Fatalf("value=%q err=%v", value, err)
	}
	var migrations int
	if err := db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&migrations); err != nil || migrations != 1 {
		t.Fatalf("migrations=%d err=%v", migrations, err)
	}
}

func TestOpenExplainsUnsupportedPrereleaseDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE schema_migrations(name TEXT PRIMARY KEY, applied_at TEXT);
		INSERT INTO schema_migrations(name,applied_at) VALUES('001_settings.sql','synthetic-time');
		CREATE TABLE account_affinity(id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = Open(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), "older pre-release schema") {
		t.Fatalf("legacy database did not explain startup failure: %v", err)
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var count int
	if err := check.QueryRow(`SELECT count(*) FROM schema_migrations WHERE name='001_schema.sql'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("legacy database was modified during rejection: count=%d err=%v", count, err)
	}
}
