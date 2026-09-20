package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/gateway"
	storedb "github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
)

func openThrough(t *testing.T, path, last string) *sql.DB {
	t.Helper()
	connection, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	connection.SetMaxOpenConns(1)
	if _, err := connection.Exec("PRAGMA foreign_keys=ON; CREATE TABLE schema_migrations(name TEXT PRIMARY KEY,applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() > last {
			break
		}
		if err := apply(context.Background(), connection, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	return connection
}
func TestConsolidationPreservesKeysPoliciesAndUsage(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	path := filepath.Join(directory, "upgrade.db")
	old := openThrough(t, path, "016_key_secrets.sql")
	for _, statement := range []string{
		"INSERT INTO users(id,username,password_hash,role,enabled,created_at) VALUES(1,'synthetic-admin','synthetic-hash','admin',1,123),(2,'synthetic-member','synthetic-hash','member',1,123),(3,'synthetic-unlimited','synthetic-hash','member',0,123)",
		"INSERT INTO group_members(group_id,user_id) VALUES(1,2),(1,3)",
		"INSERT INTO member_limits(user_id,requests_per_minute,max_concurrency) VALUES(2,17,3)",
		"INSERT INTO member_rate(user_id,window_start,requests) VALUES(2,1800000000,4)",
		"INSERT INTO settings(key,value) VALUES('synthetic-setting','unchanged')",
	} {
		if _, err := old.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	secret := "sl_" + strings.Repeat("a", 43)
	legacy := "sl_" + strings.Repeat("b", 43)
	digest := sha256.Sum256([]byte(secret))
	legacyDigest := sha256.Sum256([]byte(legacy))
	sessionDigest := bytes.Repeat([]byte{3}, 32)
	if _, err := old.Exec("INSERT INTO sessions(token_hash,user_id,created_at,expires_at) VALUES(?,2,123,1900000000)", sessionDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec("INSERT INTO api_keys(id,user_id,group_id,name,prefix,token_hash,created_at) VALUES(11,2,1,'Synthetic encrypted','sl_aaaaaaaa',?,123),(12,2,1,'Synthetic legacy','sl_bbbbbbbb',?,123)", digest[:], legacyDigest[:]); err != nil {
		t.Fatal(err)
	}
	cipher, err := vault.Open(filepath.Join(directory, "credentials.key"), true)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := cipher.SealAPIKey(2, 11, []byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec("INSERT INTO api_key_secrets(key_id,secret) VALUES(11,?)", encrypted); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().Truncate(24 * time.Hour).Add(-24 * time.Hour).Unix()
	dailySince, hourlySince := day-5*86400, day+1800
	if _, err := old.Exec("UPDATE usage_coverage SET started_at=?", dailySince); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec("UPDATE usage_hourly_coverage SET started_at=?", hourlySince); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec("INSERT INTO usage_daily(day,user_id,group_id,provider,model,requests,completed,input_tokens,input_reported) VALUES(?,2,1,'codex','synthetic-model',3,3,30,1)", day); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec("INSERT INTO usage_hourly(hour,user_id,requests,input_tokens,output_tokens,input_reported,output_reported) VALUES(?,2,3,30,0,1,0)", day+3600); err != nil {
		t.Fatal(err)
	}
	// Force a failure after ciphertext and policies have been copied, before source tables are removed.
	if _, err := old.Exec("CREATE TRIGGER fail_coverage BEFORE INSERT ON settings WHEN NEW.key='usage.hourly.started_at' BEGIN SELECT RAISE(ABORT,'synthetic migration failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err := apply(ctx, old, "017_consolidate.sql"); err == nil {
		t.Fatal("failed migration reported success")
	}
	var partial int
	if err := old.QueryRow("SELECT count(*) FROM schema_migrations WHERE name='017_consolidate.sql'").Scan(&partial); err != nil || partial != 0 {
		t.Fatal("partial migration recorded", err)
	}
	var retained []byte
	if err := old.QueryRow("SELECT secret FROM api_key_secrets WHERE key_id=11").Scan(&retained); err != nil || !bytes.Equal(retained, encrypted) {
		t.Fatal("failed migration lost ciphertext", err)
	}
	if _, err := old.Exec("DROP TRIGGER fail_coverage"); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		connection, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { connection.Close() })
		var gotCipher, gotHash, gotSession []byte
		if err := connection.QueryRow("SELECT encrypted_secret,token_hash FROM api_keys WHERE id=11").Scan(&gotCipher, &gotHash); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(encrypted, gotCipher) || !bytes.Equal(digest[:], gotHash) {
			t.Fatal("key identity or ciphertext changed")
		}
		if err := connection.QueryRow("SELECT token_hash FROM sessions WHERE user_id=2").Scan(&gotSession); err != nil || !bytes.Equal(gotSession, sessionDigest) {
			t.Fatal("session changed", err)
		}
		keys := apikey.New(connection, cipher)
		if err := keys.Verify(ctx); err != nil {
			t.Fatal(err)
		}
		if value, err := keys.Reveal(ctx, 2, 11); err != nil || value != secret {
			t.Fatal("migrated key cannot be copied", err)
		}
		for _, value := range []string{secret, legacy} {
			if principal, err := keys.Authenticate(ctx, value); err != nil || principal.UserID != 2 {
				t.Fatal("existing key invalidated", err)
			}
		}
		page, err := keys.List(ctx, 2, 0)
		if err != nil || len(page.Keys) != 2 || page.Keys[0].Copyable || !page.Keys[1].Copyable {
			t.Fatal("legacy copyability changed", err)
		}
		q := storedb.New(connection)
		for id, want := range map[int64][2]int64{1: {0, 0}, 2: {17, 3}, 3: {0, 0}} {
			var rpm, concurrency int64
			if err := connection.QueryRow("SELECT requests_per_minute,max_concurrency FROM users WHERE id=?", id).Scan(&rpm, &concurrency); err != nil || rpm != want[0] || concurrency != want[1] {
				t.Fatal("policy migration lost values/defaults", id, err)
			}
			limits, err := q.GetMemberLimits(ctx, id)
			if err != nil || limits.RequestsPerMinute != rpm || limits.MaxConcurrency != concurrency {
				t.Fatal("policy query changed", err)
			}
		}
		window, err := q.GetMemberWindow(ctx, 2)
		if err != nil || window.WindowStart != 1800000000 || window.Requests != 4 {
			t.Fatal("minute counter reset", err)
		}
		service := gateway.New(ctx, connection, nil, nil)
		stats, err := service.UserStatistics(ctx, 2, 7)
		service.Close()
		if err != nil || stats.TrackingSince != dailySince || stats.Activity.TrackingSince != hourlySince || stats.Totals.Requests != 3 || stats.Totals.InputTokens != 30 {
			t.Fatal("coverage or daily totals changed", err)
		}
		var hourlyRequests int64
		for _, cell := range stats.Activity.Cells {
			hourlyRequests += cell.Requests
		}
		if hourlyRequests != 3 {
			t.Fatal("hourly totals changed")
		}
		var setting string
		if err := connection.QueryRow("SELECT value FROM settings WHERE key='synthetic-setting'").Scan(&setting); err != nil || setting != "unchanged" {
			t.Fatal("unrelated setting changed", err)
		}
		var oldTables int
		if err := connection.QueryRow("SELECT count(*) FROM sqlite_schema WHERE type='table' AND name IN ('api_key_secrets','member_limits','usage_coverage','usage_hourly_coverage')").Scan(&oldTables); err != nil || oldTables != 0 {
			t.Fatal("superseded storage remains", oldTables, err)
		}
		rows, err := connection.Query("PRAGMA foreign_key_check")
		if err != nil {
			t.Fatal(err)
		}
		if rows.Next() {
			t.Fatal("foreign key violation")
		}
		rows.Close()
		if err := connection.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
