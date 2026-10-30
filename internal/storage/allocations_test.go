package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	storedb "github.com/murongg/SubLane/internal/storage/db"
)

func TestAllocationRevisionMigrationPreservesLedgerAndSequence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic-allowance-upgrade.db")
	previous, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := previous.Exec(`CREATE TABLE schema_migrations(name TEXT PRIMARY KEY NOT NULL,applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Name() >= "025_allocation_anchors.sql" {
			break
		}
		if err := apply(ctx, previous, file.Name()); err != nil {
			t.Fatal(err)
		}
	}
	config := `{"mode":"tokens","period":"month","members":[{"user_id":1,"limit":100}],"rates":[]}`
	for _, statement := range []string{
		`INSERT INTO users(id,username,password_hash,role,created_at) VALUES(1,'synthetic-owner','synthetic-hash','admin',1)`,
		`INSERT INTO tenants(id,name,owner_user_id,created_at) VALUES(1,'Synthetic workspace',1,1)`,
		`INSERT INTO account_groups(id,name,enabled,created_at,updated_at) VALUES(1,'Synthetic pool',1,1,1)`,
		`INSERT INTO allocation_schemes(id,name,group_id,enabled,created_at) VALUES(1,'Synthetic budget',1,1,1)`,
		`INSERT INTO api_keys(id,user_id,group_id,name,prefix,token_hash,created_at) VALUES(1,1,1,'Synthetic key','synthetic',zeroblob(32),1)`,
		`INSERT INTO allocation_keys(key_id,scheme_id) VALUES(1,1)`,
	} {
		if _, err := previous.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for _, revision := range []struct{ id, at int }{{11, 100}, {12, 200}, {99, 300}} {
		if _, err := previous.Exec(`INSERT INTO allocation_revisions(id,scheme_id,effective_at,config) VALUES(?,1,?,?)`, revision.id, revision.at, config); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := previous.Exec(`DELETE FROM allocation_revisions WHERE id=99;
		INSERT INTO allocation_entries(request_id,scheme_id,revision_id,user_id,account_id,model,mode,window_start,reset_at,started_at,state,cost)
		VALUES('synthetic-settled',1,11,1,'synthetic-account','synthetic-model','tokens',100,200,110,'settled',3),
		('synthetic-pending',1,11,1,'synthetic-account','synthetic-model','tokens',100,200,120,'pending',0)`); err != nil {
		t.Fatal(err)
	}
	previous.Close()
	for range 2 {
		conn, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		var saved string
		var at, anchor, entries, key int64
		err = conn.QueryRow(`SELECT effective_at,accounting_anchor_at,config FROM allocation_revisions WHERE id=11`).Scan(&at, &anchor, &saved)
		if err != nil || at != 100 || anchor != at || saved != config {
			t.Fatal("current revision changed", at, anchor, saved, err)
		}
		if err := conn.QueryRow(`SELECT accounting_anchor_at FROM allocation_revisions WHERE id=12`).Scan(&anchor); err != nil || anchor != 200 {
			t.Fatal("scheduled anchor changed", anchor, err)
		}
		if err := conn.QueryRow(`SELECT count(*) FROM allocation_entries WHERE revision_id=11`).Scan(&entries); err != nil || entries != 2 {
			t.Fatal("request references lost", entries, err)
		}
		if err := conn.QueryRow(`SELECT scheme_id FROM allocation_keys WHERE key_id=1`).Scan(&key); err != nil || key != 1 {
			t.Fatal("key binding changed", key, err)
		}
		var invalid int
		if err := conn.QueryRow(`SELECT count(*) FROM pragma_foreign_key_check`).Scan(&invalid); err != nil || invalid != 0 {
			t.Fatal("invalid foreign keys", invalid, err)
		}
		conn.Close()
	}
	conn, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	result, err := conn.Exec(`INSERT INTO allocation_revisions(scheme_id,effective_at,accounting_anchor_at,config) VALUES(1,100,100,?)`, config)
	if err != nil {
		t.Fatal("same-second revision rejected", err)
	}
	id, err := result.LastInsertId()
	if err != nil || id <= 99 {
		t.Fatal("revision sequence reused a retired ID", id, err)
	}
	current, err := storedb.New(conn).CurrentAllocationRevision(ctx, storedb.CurrentAllocationRevisionParams{SchemeID: 1, EffectiveAt: 100})
	if err != nil || current.ID != id {
		t.Fatal("same-second current selection is not deterministic", current.ID, id, err)
	}
	result, err = conn.Exec(`INSERT INTO allocation_revisions(scheme_id,effective_at,accounting_anchor_at,config) VALUES(1,200,200,?)`, config)
	if err != nil {
		t.Fatal(err)
	}
	id, _ = result.LastInsertId()
	next, err := storedb.New(conn).NextAllocationRevision(ctx, storedb.NextAllocationRevisionParams{SchemeID: 1, EffectiveAt: 100})
	if err != nil || next.ID != id {
		t.Fatal("same-second next selection is not deterministic", next.ID, id, err)
	}
	target := filepath.Join(t.TempDir(), "synthetic-restored.db")
	if err := Snapshot(ctx, path, target, 8<<20); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var retained int
	if err := restored.QueryRow(`SELECT count(*) FROM allocation_entries WHERE revision_id=11`).Scan(&retained); err != nil || retained != 2 {
		t.Fatal("snapshot lost ledger references", retained, err)
	}
}
