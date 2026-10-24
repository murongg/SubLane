package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestKeyStatisticsMigrationPreservesLegacyUsage(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic-statistics.db")
	previous, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer previous.Close()
	if _, err := previous.Exec(`CREATE TABLE schema_migrations(name TEXT PRIMARY KEY NOT NULL,applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() >= "021_key_usage.sql" {
			break
		}
		if err := apply(ctx, previous, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		`INSERT INTO users(id,username,password_hash,role,created_at) VALUES(1,'synthetic-owner','synthetic-hash','admin',1)`,
		`INSERT INTO tenants(id,name,owner_user_id,created_at) VALUES(1,'Synthetic workspace',1,1)`,
		`INSERT INTO usage_daily(day,user_id,group_id,provider,model,requests,completed,incomplete,errors,canceled,rejected,duration_ms,input_tokens,output_tokens,cached_tokens,input_reported,output_reported,cached_reported) VALUES(86400,1,1,'codex','synthetic',5,1,1,1,1,1,120,40,20,10,3,2,1)`,
		`INSERT INTO usage_hourly(tenant_id,hour,user_id,requests,input_tokens,output_tokens,input_reported,output_reported) VALUES(1,86400,1,5,40,20,3,2)`,
		`UPDATE settings SET value='1' WHERE key IN ('usage.daily.started_at','usage.hourly.started_at')`,
	} {
		if _, err := previous.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := previous.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var keyID, requests, input, output, cached, completed, incomplete, failures, canceled, rejected, duration, inputReported, outputReported, cachedReported int64
	err = upgraded.QueryRow(`SELECT key_id,requests,input_tokens,output_tokens,cached_tokens,completed,incomplete,errors,canceled,rejected,duration_ms,input_reported,output_reported,cached_reported FROM usage_daily`).Scan(&keyID, &requests, &input, &output, &cached, &completed, &incomplete, &failures, &canceled, &rejected, &duration, &inputReported, &outputReported, &cachedReported)
	if err != nil || keyID != 0 || requests != 5 || input != 40 || output != 20 || cached != 10 || completed != 1 || incomplete != 1 || failures != 1 || canceled != 1 || rejected != 1 || duration != 120 || inputReported != 3 || outputReported != 2 || cachedReported != 1 {
		t.Fatal("legacy daily usage changed", err)
	}
	err = upgraded.QueryRow(`SELECT key_id,requests,input_tokens,output_tokens,input_reported,output_reported FROM usage_hourly`).Scan(&keyID, &requests, &input, &output, &inputReported, &outputReported)
	if err != nil || keyID != 0 || requests != 5 || input != 40 || output != 20 || inputReported != 3 || outputReported != 2 {
		t.Fatal("legacy hourly usage changed", err)
	}
	var since, dailySince int64
	if err := upgraded.QueryRow(`SELECT value FROM settings WHERE key='usage.keys.started_at'`).Scan(&since); err != nil || since <= 1 {
		t.Fatal("missing independent key coverage", since, err)
	}
	if err := upgraded.QueryRow(`SELECT value FROM settings WHERE key='usage.daily.started_at'`).Scan(&dailySince); err != nil || dailySince != 1 {
		t.Fatal("global coverage changed", dailySince, err)
	}
	for _, id := range []int{1, 2} {
		if _, err := upgraded.Exec(`INSERT INTO usage_daily(day,user_id,group_id,provider,model,key_id,requests) VALUES(86400,1,1,'codex','synthetic',?,1)`, id); err != nil {
			t.Fatal("daily keys share a bucket", err)
		}
		if _, err := upgraded.Exec(`INSERT INTO usage_hourly(tenant_id,hour,user_id,key_id,requests,input_tokens,output_tokens,input_reported,output_reported) VALUES(1,86400,1,?,1,0,0,0,0)`, id); err != nil {
			t.Fatal("hourly keys share a bucket", err)
		}
	}
	for _, statement := range []string{
		`UPDATE usage_daily SET input_tokens=-1`,
		`UPDATE usage_daily SET model=printf('%181s','x')`,
		`UPDATE usage_hourly SET input_reported=requests+1`,
		`UPDATE usage_hourly SET tenant_id=999`,
	} {
		if _, err := upgraded.Exec(statement); err == nil {
			t.Fatal("migration lost a constraint", statement)
		}
	}
	var count int
	if err := upgraded.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name IN ('usage_daily_user','usage_hourly_user','usage_hourly_retention')`).Scan(&count); err != nil || count != 3 {
		t.Fatal("migration lost query/retention indexes", count, err)
	}
	if err := upgraded.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var retainedSince int64
	if err := reopened.QueryRow(`SELECT value FROM settings WHERE key='usage.keys.started_at'`).Scan(&retainedSince); err != nil || retainedSince != since {
		t.Fatal("restart reset key tracking", retainedSince, err)
	}
}
