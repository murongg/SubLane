package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

const syntheticBackupPassword = "synthetic backup passphrase"

func TestNewBackupsAreEncrypted(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(directory, databaseName))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := vault.Open(filepath.Join(directory, keyName), true); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "synthetic.tar.gz.age")
	if _, err := Create(ctx, directory, output, "synthetic-version", Options{Password: syntheticBackupPassword}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte("age-encryption.org/v1\n")) {
		t.Fatal("backup was published without age encryption")
	}
}

func TestEncryptedBackupPasswordsIntegrityAndExternalKey(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(directory, databaseName))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	keyFile := filepath.Join(t.TempDir(), "credentials.key")
	if _, err := vault.Open(keyFile, true); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "synthetic.age")
	if _, err := Create(ctx, directory, output, "synthetic-version", Options{KeyFile: keyFile}); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("unencrypted export allowed: %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("failed export published output", err)
	}
	info, err := Create(ctx, directory, output, "synthetic-version", Options{KeyFile: keyFile, Password: syntheticBackupPassword})
	if err != nil || !info.Encrypted {
		t.Fatalf("external key export: %+v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(directory, keyName)); !os.IsNotExist(err) {
		t.Fatal("external key was copied into data directory", err)
	}
	if _, err := Verify(ctx, output, ""); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("missing password: %v", err)
	}
	if _, err := Verify(ctx, output, "incorrect synthetic passphrase"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("wrong password: %v", err)
	}
	if verified, err := Verify(ctx, output, syntheticBackupPassword); err != nil || !verified.Encrypted {
		t.Fatalf("encrypted verify: %+v %v", verified, err)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"tampered", "truncated", "appended"} {
		t.Run(kind, func(t *testing.T) {
			bad := append([]byte(nil), raw...)
			switch kind {
			case "tampered":
				bad[len(bad)-1] ^= 1
			case "truncated":
				bad = bad[:len(bad)-1]
			case "appended":
				bad = append(bad, []byte("hidden synthetic bytes")...)
			}
			path := filepath.Join(t.TempDir(), "bad.age")
			if err := os.WriteFile(path, bad, 0600); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "restored")
			if _, err := Restore(ctx, path, target, syntheticBackupPassword); err == nil {
				t.Fatal("corrupted ciphertext was accepted")
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("failed decryption published restored data", err)
			}
		})
	}
	target := filepath.Join(t.TempDir(), "restored")
	if _, err := Restore(ctx, output, target, syntheticBackupPassword); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Open(filepath.Join(target, keyName), false); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyArchiveStillVerifiesWithoutPassword(t *testing.T) {
	_, entries := archiveFixture(t)
	path := filepath.Join(t.TempDir(), "legacy.tar.gz")
	if err := os.WriteFile(path, encodeArchive(t, entries), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := Verify(context.Background(), path, "")
	if err != nil || info.Encrypted {
		t.Fatalf("legacy archive detection: %+v %v", info, err)
	}
}

func TestBackupPasswordPolicyAndExpensiveImportedKDF(t *testing.T) {
	for _, password := range []string{"short", strings.Repeat("x", 129), "synthetic pass\nphrase"} {
		if _, err := Create(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "backup.age"), "synthetic", Options{Password: password}); !errors.Is(err, ErrPassword) {
			t.Fatalf("weak or unsupported export password accepted: %v", err)
		}
	}
	// A forged header is rejected before attempting the advertised, more expensive scrypt work.
	raw := []byte("age-encryption.org/v1\n-> scrypt AAAAAAAAAAAAAAAAAAAAAA 19\nAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n--- AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n")
	path := filepath.Join(t.TempDir(), "expensive.age")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(context.Background(), path, syntheticBackupPassword); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("unsupported KDF header accepted: %v", err)
	}
}
