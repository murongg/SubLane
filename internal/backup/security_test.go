package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
	"github.com/pquerna/otp/totp"
)

func TestBackupVerifiesTOTPSecrets(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	conn, err := storage.Open(ctx, filepath.Join(dir, databaseName))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	path := filepath.Join(dir, keyName)
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
	codes, err := s.EnableTOTP(ctx, session.Token, setup.Challenge, code)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyCredentials(ctx, conn, filepath.Join(dir, keyName)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.BeginPasskeyLogin(ctx, "https://example.test"); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "synthetic.tar.gz")
	if _, err := Create(ctx, dir, archive, "synthetic-version", Options{Password: syntheticBackupPassword}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "restored")
	if _, err := Restore(ctx, archive, target, syntheticBackupPassword); err != nil {
		t.Fatal(err)
	}
	restored, err := storage.Open(ctx, filepath.Join(target, databaseName))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var challenges int
	if err := restored.QueryRow("SELECT count(*) FROM auth_challenges").Scan(&challenges); err != nil || challenges != 0 {
		t.Fatalf("restored pending authentication: %d %v", challenges, err)
	}
	restoredCipher, err := vault.Open(filepath.Join(target, keyName), false)
	if err != nil {
		t.Fatal(err)
	}
	restoredAuth, err := auth.NewWithVault(restored, restoredCipher)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restoredAuth.LoginWithCode(ctx, "synthetic-owner", "synthetic-password", codes[0]); err != nil {
		t.Fatal("restored recovery code unusable", err)
	}
	if err := os.WriteFile(path, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyCredentials(ctx, conn, filepath.Join(dir, keyName)); err == nil {
		t.Fatal("backup accepted invalid TOTP encryption key")
	}
}
