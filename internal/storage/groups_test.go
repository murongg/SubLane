package storage

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestGroupMigrationPreservesExistingKeysAndAffinity(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	connection, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	if _, err = connection.Exec("PRAGMA foreign_keys=ON; CREATE TABLE schema_migrations(name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	entries, _ := migrations.ReadDir("migrations")
	for _, entry := range entries {
		if entry.Name() >= "008_groups.sql" {
			break
		}
		if err := apply(ctx, connection, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	digest := bytes.Repeat([]byte{1}, 32)
	for _, q := range []string{
		"INSERT INTO users(id,username,password_hash,role,enabled,created_at) VALUES(1,'synthetic-admin','synthetic-hash','admin',1,1),(2,'synthetic-member','synthetic-hash','member',1,1)",
		"INSERT INTO accounts(id,provider,name,account_id,credential,status,created_at,updated_at) VALUES('synthetic-account','codex','Synthetic','synthetic-upstream',x'1234','ready',1,1)",
	} {
		if _, err = connection.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = connection.Exec("INSERT INTO api_keys(id,user_id,name,prefix,token_hash,created_at) VALUES(1,2,'Synthetic key','sl_synthetic',?,1)", digest); err != nil {
		t.Fatal(err)
	}
	if _, err = connection.Exec("INSERT INTO account_affinity(user_id,session_hash,provider,account_id,expires_at) VALUES(2,?,'codex','synthetic-account',2000000000)", digest); err != nil {
		t.Fatal(err)
	}
	connection.Close()
	connection, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var groupID int64
	var saved []byte
	if err = connection.QueryRow("SELECT group_id,token_hash FROM api_keys WHERE id=1").Scan(&groupID, &saved); err != nil || groupID != 1 || !bytes.Equal(saved, digest) {
		t.Fatal("legacy key changed", err)
	}
	if err = connection.QueryRow("SELECT group_id,session_hash FROM account_affinity WHERE user_id=2").Scan(&groupID, &saved); err != nil || groupID != 1 || !bytes.Equal(saved, digest) {
		t.Fatal("legacy affinity changed", err)
	}
	var count int
	if err = connection.QueryRow("SELECT count(*) FROM group_accounts WHERE group_id=1 AND account_id='synthetic-account'").Scan(&count); err != nil || count != 1 {
		t.Fatal("default pool missing", err)
	}
	if err = connection.QueryRow("SELECT count(*) FROM group_members WHERE group_id=1 AND user_id=2").Scan(&count); err != nil || count != 1 {
		t.Fatal("legacy member lost access", err)
	}
}
