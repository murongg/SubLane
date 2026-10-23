package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
	"github.com/pquerna/otp/totp"
)

func TestMissingVaultKeyIsNotRecreatedForTOTP(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	conn, err := storage.Open(ctx, filepath.Join(dir, "sublane.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	path := filepath.Join(dir, "credentials.key")
	cipher, err := vault.Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	s, err := auth.NewWithVault(conn, cipher)
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.Setup(ctx, "synthetic-owner", "synthetic-password", "Synthetic")
	if err != nil {
		t.Fatal(err)
	}
	setup, err := s.BeginTOTP(ctx, session.Token, auth.Proof{Password: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	if _, err := s.EnableTOTP(ctx, session.Token, setup.Challenge, code); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := openVault(ctx, conn, dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recreated lost TOTP key: %v", err)
	}
}
