package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenPreservesRequestHistoryWithoutInventingRouting(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic-route-upgrade.db")
	previous, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := previous.Exec(`CREATE TABLE schema_migrations(name TEXT PRIMARY KEY NOT NULL,applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() >= "025_route_explanation.sql" {
			break
		}
		if err := apply(ctx, previous, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := previous.Exec(`INSERT INTO request_records(user_id,key_id,group_id,transport,operation,started_at,duration_ms,outcome,model) VALUES(1,1,1,'http','responses',123,10,'success','synthetic-model')`); err != nil {
		t.Fatal(err)
	}
	previous.Close()
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var model, route string
	if err := upgraded.QueryRow(`SELECT model,route_explanation FROM request_records`).Scan(&model, &route); err != nil || model != "synthetic-model" || route != "null" {
		t.Fatal("upgrade rewrote historical routing or lost history", model, route, err)
	}
	for _, raw := range []string{"invalid-json", `"` + strings.Repeat("x", 1024) + `"`} {
		if _, err := upgraded.Exec(`UPDATE request_records SET route_explanation=?`, raw); err == nil {
			t.Fatal("invalid or unbounded route metadata accepted")
		}
	}
}
