package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestMissingVaultKeyIsNotRecreatedForGatewaySecrets(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "sublane.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	identity, err := auth.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "synthetic-admin", "synthetic-pass", "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials.key")
	cipher, err := vault.Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec("INSERT INTO account_groups(id,name,enabled,created_at,updated_at) VALUES(1,'Synthetic pool',1,1,1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := apikey.New(connection, cipher).CreateInGroup(ctx, 1, 1, "Synthetic key"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := openVault(ctx, connection, dir); err == nil {
		t.Fatal("lost encryption key silently replaced")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("new encryption file was created")
	}
}
