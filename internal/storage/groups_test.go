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

func TestPoolUpgradeAllowsEditsWithoutChangingBindings(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic-pools.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	defer conn.Close()
	if _, err = conn.Exec("PRAGMA foreign_keys=ON; CREATE TABLE schema_migrations(name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	entries, _ := migrations.ReadDir("migrations")
	for _, entry := range entries {
		if entry.Name() > "023_team_access.sql" {
			break
		}
		if err = apply(ctx, conn, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		"INSERT INTO users(id,username,password_hash,role,enabled,created_at) VALUES(1,'synthetic-admin','synthetic','admin',1,1),(2,'synthetic-member','synthetic','member',1,1)",
		"INSERT INTO accounts(id,provider,name,account_id,credential,status,created_at,updated_at) VALUES('synthetic-account','codex','Synthetic','synthetic-upstream',x'1234','ready',1,1)",
		"INSERT INTO group_accounts VALUES(1,'synthetic-account')",
		"INSERT INTO group_members VALUES(1,2)",
		"UPDATE account_groups SET restricted_models=1 WHERE id=1",
		"INSERT INTO group_models VALUES(1,'synthetic-model')",
		"INSERT INTO api_keys(id,user_id,group_id,name,prefix,token_hash,created_at) VALUES(1,2,1,'Synthetic','sl_synthetic',zeroblob(32),1)",
		"INSERT INTO account_affinity(user_id,group_id,session_hash,provider,account_id,expires_at) VALUES(2,1,zeroblob(32),'codex','synthetic-account',2000000000)",
	} {
		if _, err = conn.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if err = migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"group_accounts", "group_members", "group_models", "api_keys", "account_affinity"} {
		var count int
		if err = conn.QueryRow("SELECT count(*) FROM " + table + " WHERE group_id=1").Scan(&count); err != nil || count != 1 {
			t.Fatalf("lost %s: %d %v", table, count, err)
		}
	}
	if _, err = conn.Exec("UPDATE account_groups SET name='Synthetic migrated pool',enabled=0 WHERE id=1"); err != nil {
		t.Fatalf("old default is still protected: %v", err)
	}
	var restricted, foreignKeys int
	if err = conn.QueryRow("SELECT restricted_models FROM account_groups WHERE id=1").Scan(&restricted); err != nil || restricted != 1 {
		t.Fatal("model policy changed", err)
	}
	if err = conn.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatal("foreign key enforcement disabled", err)
	}
	if _, err = conn.Exec("INSERT INTO group_accounts VALUES(999,'synthetic-account')"); err == nil {
		t.Fatal("foreign key enforcement bypassed")
	}
	if err = migrate(ctx, conn); err != nil {
		t.Fatal("migration not idempotent", err)
	}
}

func TestPoolUpgradeRollsBackOnInvalidReferences(t *testing.T) {
	ctx := context.Background()
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "synthetic-broken-pool.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	if _, err = conn.Exec("PRAGMA foreign_keys=ON; CREATE TABLE schema_migrations(name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	entries, _ := migrations.ReadDir("migrations")
	for _, entry := range entries {
		if entry.Name() > "023_team_access.sql" {
			break
		}
		if err = apply(ctx, conn, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate damage from an external writer with FK enforcement disabled.
	if _, err = conn.Exec("PRAGMA foreign_keys=OFF; INSERT INTO group_models VALUES(999,'synthetic-orphan'); PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	if err = apply(ctx, conn, "024_pools.sql"); err == nil {
		t.Fatal("invalid references committed")
	}
	var enabled, applied int
	if err = conn.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil || enabled != 1 {
		t.Fatal("failure left FK enforcement disabled", err)
	}
	if err = conn.QueryRow("SELECT count(*) FROM schema_migrations WHERE name='024_pools.sql'").Scan(&applied); err != nil || applied != 0 {
		t.Fatal("failed migration marked applied", err)
	}
	if _, err = conn.Exec("UPDATE account_groups SET name='Synthetic changed' WHERE id=1"); err == nil {
		t.Fatal("failed migration did not roll back old schema")
	}
	if _, err = conn.Exec("DELETE FROM group_models WHERE group_id=999"); err != nil {
		t.Fatal(err)
	}
	if err = apply(ctx, conn, "024_pools.sql"); err != nil {
		t.Fatal("could not retry after repair", err)
	}
}
