package storage

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestProviderMigrationPreservesCredentialsSnapshotsAndBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	connection, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	connection.SetMaxOpenConns(1)
	if _, err = connection.Exec("PRAGMA foreign_keys=ON; CREATE TABLE schema_migrations(name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	entries, _ := migrations.ReadDir("migrations")
	for _, entry := range entries {
		if entry.Name() >= "007_providers.sql" {
			break
		}
		if err := apply(ctx, connection, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = connection.Exec("INSERT INTO users(id,username,password_hash,role,enabled,created_at) VALUES (1,'synthetic-owner','synthetic-hash','admin',1,1)"); err != nil {
		t.Fatal(err)
	}
	encrypted := []byte("synthetic-encrypted-bytes")
	if _, err = connection.Exec("INSERT INTO accounts(id,name,account_id,credential,status,created_at,updated_at) VALUES ('synthetic-account','Synthetic','synthetic-upstream',?,'ready',1,1)", encrypted); err != nil {
		t.Fatal(err)
	}
	if _, err = connection.Exec("INSERT INTO account_usage(account_id,snapshot,updated_at) VALUES ('synthetic-account',?,1)", []byte(`{"limits":[],"updated_at":1}`)); err != nil {
		t.Fatal(err)
	}
	hash := bytes.Repeat([]byte{1}, 32)
	if _, err = connection.Exec("INSERT INTO account_affinity(user_id,session_hash,account_id,expires_at) VALUES (1,?,'synthetic-account',2000000000)", hash); err != nil {
		t.Fatal(err)
	}
	connection.Close()
	connection, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var provider string
	var credential []byte
	if err = connection.QueryRow("SELECT provider,credential FROM accounts WHERE id='synthetic-account'").Scan(&provider, &credential); err != nil || provider != "codex" || !bytes.Equal(encrypted, credential) {
		t.Fatal("credential migration changed stored data", err)
	}
	var count int
	if err = connection.QueryRow("SELECT count(*) FROM account_usage WHERE account_id='synthetic-account'").Scan(&count); err != nil || count != 1 {
		t.Fatal("snapshot was cascaded away", err)
	}
	if err = connection.QueryRow("SELECT provider FROM account_affinity WHERE user_id=1 AND session_hash=?", hash).Scan(&provider); err != nil || provider != "codex" {
		t.Fatal("legacy affinity was lost", err)
	}
	if _, err = connection.Exec("INSERT INTO accounts(id,provider,name,account_id,credential,status,created_at,updated_at) VALUES ('synthetic-claude','claude','Synthetic','synthetic-upstream',?,'ready',1,1)", encrypted); err != nil {
		t.Fatal("cross-provider subject collision", err)
	}
	if _, err = connection.Exec("DELETE FROM accounts WHERE id='synthetic-account'"); err != nil {
		t.Fatal(err)
	}
	if err = connection.QueryRow("SELECT count(*) FROM account_usage").Scan(&count); err != nil || count != 0 {
		t.Fatal("cascade no longer works", err)
	}
}
