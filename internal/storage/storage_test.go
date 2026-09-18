package storage

import (
	"bytes"
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
	if _, err = db.Exec("SELECT id FROM users"); err != nil {
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
	if err := db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&migrations); err != nil || migrations != 4 {
		t.Fatalf("migrations=%d err=%v", migrations, err)
	}
}

func TestUpgradePreservesAdministratorAndSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth-upgrade.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE schema_migrations(name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, name := range []string{"001_settings.sql", "002_auth.sql"} {
		if err := apply(ctx, db, name); err != nil {
			t.Fatal(err)
		}
	}
	digest := bytes.Repeat([]byte{42}, 32)
	if _, err := db.Exec("INSERT INTO administrators VALUES(1, 'owner-test', 'synthetic-hash', 123)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO admin_sessions VALUES(?, 1, 123, 9999999999)", digest); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var username, role, hash string
	var enabled bool
	if err := db.QueryRow("SELECT username, role, password_hash, enabled FROM users WHERE id=1").Scan(&username, &role, &hash, &enabled); err != nil {
		t.Fatal(err)
	}
	if username != "owner-test" || role != "admin" || hash != "synthetic-hash" || !enabled {
		t.Fatal("administrator changed during migration")
	}
	var token []byte
	var userID, created, expires int64
	if err := db.QueryRow("SELECT token_hash, user_id, created_at, expires_at FROM sessions").Scan(&token, &userID, &created, &expires); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(token, digest) || userID != 1 || created != 123 || expires != 9999999999 {
		t.Fatal("session changed during migration")
	}
	if _, err := db.Exec("UPDATE users SET enabled=0 WHERE id=1"); err == nil {
		t.Fatal("owner can be disabled")
	}
}
