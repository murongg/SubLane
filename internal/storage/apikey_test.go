package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	queries "github.com/murongg/SubLane/internal/storage/db"
)

func TestAPIKeyMigrationPreservesReferences(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.db")
	previous, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := previous.Exec(`CREATE TABLE schema_migrations(name TEXT PRIMARY KEY NOT NULL,applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	entries, _ := migrations.ReadDir("migrations")
	for _, entry := range entries {
		if entry.Name() >= "015_api_upstreams.sql" {
			break
		}
		if err := apply(ctx, previous, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		`INSERT INTO users(id,username,password_hash,role,created_at) VALUES(1,'synthetic-owner','synthetic-hash','admin',1)`,
		`INSERT INTO tenants(id,name,owner_user_id,created_at) VALUES(1,'Synthetic workspace',1,1)`,
		`INSERT INTO accounts(id,name,account_id,status,credential,created_at,updated_at) VALUES('synthetic-account','Synthetic account','synthetic-subject','ready',X'1234',1,1)`,
		`INSERT INTO account_groups(id,name,enabled,created_at,updated_at) VALUES(1,'Synthetic pool',1,1,1)`,
		`INSERT INTO group_accounts(group_id,account_id) VALUES(1,'synthetic-account')`,
		`INSERT INTO account_usage(account_id,snapshot,updated_at) VALUES('synthetic-account',X'1234',1)`,
		`INSERT INTO account_model_runtime(account_id,model,cooldown_until,failures,lifecycle,runtime_revision) VALUES('synthetic-account','synthetic-model',10,1,1,1)`,
	} {
		if _, err := previous.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	previous.Close()
	connection, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	for _, table := range []string{"accounts", "group_accounts", "account_usage", "account_model_runtime"} {
		var count int
		if err := connection.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatal("lost reference", table, count, err)
		}
	}
	if _, err := connection.Exec(`UPDATE accounts SET provider='openai'`); err != nil {
		t.Fatal("API upstream schema", err)
	}
	setup, err := queries.New(connection).GetSetupAccounts(ctx, 1)
	if err != nil || setup.Verified != 1 {
		t.Fatal("API upstream excluded from setup", setup, err)
	}
	for _, statement := range []string{`UPDATE accounts SET tenant_id=2`, `UPDATE accounts SET provider='unknown'`, `INSERT INTO group_accounts(group_id,account_id) VALUES(1,'missing')`} {
		if _, err := connection.Exec(statement); err == nil {
			t.Fatal("constraint lost", statement)
		}
	}
	var enabled int
	if err := connection.QueryRow(`PRAGMA foreign_keys`).Scan(&enabled); err != nil || enabled != 1 {
		t.Fatal("foreign keys disabled", err)
	}
}
