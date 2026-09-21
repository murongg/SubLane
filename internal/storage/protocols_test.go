package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestNativeProtocolMigrationPreservesHistoryAndSequence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "protocol-upgrade.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec("CREATE TABLE schema_migrations(name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() <= "019_request_diagnostics.sql" {
			if err := apply(ctx, old, entry.Name()); err != nil {
				t.Fatal(err)
			}
		}
	}
	insert := `INSERT INTO request_records(id,user_id,key_id,group_id,transport,operation,started_at,duration_ms,outcome,request_id,first_token_ms,input_tokens) VALUES(?,1,1,1,'http','responses',123,45,'success','req_synthetic',12,34)`
	for _, id := range []int{7, 100} {
		if _, err := old.Exec(insert, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := old.Exec("DELETE FROM request_records WHERE id=100"); err != nil {
		t.Fatal(err)
	}
	old.Close()
	connection, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var requestID string
	var first, input int64
	if err := connection.QueryRow("SELECT request_id,first_token_ms,input_tokens FROM request_records WHERE id=7").Scan(&requestID, &first, &input); err != nil || requestID != "req_synthetic" || first != 12 || input != 34 {
		t.Fatal("history changed", err)
	}
	for _, operation := range []string{"messages", "gemini"} {
		result, err := connection.Exec(`INSERT INTO request_records(user_id,key_id,group_id,transport,operation,started_at,duration_ms,outcome) VALUES(1,1,1,'http',?,123,45,'success')`, operation)
		if err != nil {
			t.Fatal("native operation rejected", err)
		}
		id, _ := result.LastInsertId()
		if id <= 100 {
			t.Fatal("request IDs reused after migration", id)
		}
	}
	var indexes int
	if err := connection.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='index' AND name IN ('request_records_time','request_records_account','request_records_request_id')").Scan(&indexes); err != nil || indexes != 3 {
		t.Fatal("history indexes lost", err)
	}
}
