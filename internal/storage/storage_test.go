package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestUpgradePreservesFoundationSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("CREATE TABLE schema_migrations(name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP); INSERT INTO schema_migrations(name) VALUES ('001_settings.sql'); CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT NOT NULL,updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP); INSERT INTO settings(key,value) VALUES ('synthetic-setting','synthetic-value')")
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	db, err = Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err = db.QueryRow("SELECT value FROM settings WHERE key='synthetic-setting'").Scan(&value); err != nil || value != "synthetic-value" {
		t.Fatalf("upgrade lost settings: %v", err)
	}
	if _, err = db.Exec("SELECT id FROM administrators"); err != nil {
		t.Fatal("auth migration missing", err)
	}
}

func TestOpenMigratesAndPreservesData(t *testing.T) {
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
	if err := db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&migrations); err != nil || migrations != 2 {
		t.Fatalf("migrations=%d err=%v", migrations, err)
	}
}
