package storage

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
)

func TestCatalogUpgradePreservesAccountsAndStartsUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog-upgrade.db")
	previous := openThrough(t, path, "017_consolidate.sql")
	cipher := []byte("synthetic-encrypted-credential")
	if _, err := previous.Exec(`INSERT INTO accounts(id,provider,name,account_id,enabled,status,credential,created_at,updated_at) VALUES('synthetic-id','codex','Synthetic','synthetic-upstream',0,'ready',?,100,200)`, cipher); err != nil {
		t.Fatal(err)
	}
	previous.Close()
	current, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	var saved, snapshot []byte
	var enabled, revision, created, updated int64
	if err := current.QueryRow(`SELECT credential,enabled,created_at,updated_at,models_snapshot,models_revision FROM accounts WHERE id='synthetic-id'`).Scan(&saved, &enabled, &created, &updated, &snapshot, &revision); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, cipher) || enabled != 0 || created != 100 || updated != 200 || snapshot != nil || revision != 0 {
		t.Fatal("catalog migration changed account state or invented capabilities")
	}
}
