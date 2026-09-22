package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestBudgetUpgradePreservesMemberLimitsAndStartsUnlimited(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budget-upgrade.db")
	previous := openThrough(t, path, "020_native_protocols.sql")
	if _, err := previous.Exec(`INSERT INTO users(id,username,role,password_hash,enabled,created_at,requests_per_minute,max_concurrency) VALUES(2,'synthetic-member','member','synthetic-hash',1,1,20,2)`); err != nil {
		t.Fatal(err)
	}
	previous.Close()
	current, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	var rpm, concurrency, rules int64
	if err := current.QueryRow("SELECT requests_per_minute,max_concurrency FROM users WHERE id=2").Scan(&rpm, &concurrency); err != nil || rpm != 20 || concurrency != 2 {
		t.Fatal("changed existing limits", rpm, concurrency, err)
	}
	if err := current.QueryRow("SELECT count(*) FROM token_budgets").Scan(&rules); err != nil || rules != 0 {
		t.Fatal("upgrade imposed quotas", rules, err)
	}
}
