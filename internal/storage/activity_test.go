package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestHourlyMigrationPreservesDailyTotalsWithoutInventingHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "activity.db")
	connection, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec("DROP TABLE usage_hourly; DROP TABLE usage_hourly_coverage; DELETE FROM schema_migrations WHERE name='012_hourly_usage.sql'; INSERT INTO usage_daily(day,user_id,group_id,provider,model,requests) VALUES(1900000000,1,1,'codex','synthetic-model',73)"); err != nil {
		connection.Close()
		t.Fatal(err)
	}
	connection.Close()
	before := time.Now().Unix()
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var count, requests, since int64
	if err := upgraded.QueryRow("SELECT COUNT(*) FROM usage_hourly").Scan(&count); err != nil || count != 0 {
		t.Fatal("fabricated hourly backfill", count, err)
	}
	if err := upgraded.QueryRow("SELECT SUM(requests) FROM usage_daily").Scan(&requests); err != nil || requests != 73 {
		t.Fatal("daily history changed", requests, err)
	}
	if err := upgraded.QueryRow("SELECT started_at FROM usage_hourly_coverage").Scan(&since); err != nil || since < before {
		t.Fatal("hourly coverage did not start at migration", since, err)
	}
}
