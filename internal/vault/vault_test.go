package vault

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptionBindsCredentialsToAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.key")
	v, err := Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte(`{"access_token":"synthetic-access","refresh_token":"synthetic-refresh"}`)
	encrypted, err := v.Seal("account-test", plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, plain) {
		t.Fatal("plaintext persisted")
	}
	second, err := v.Seal("account-test", plain)
	if err != nil || bytes.Equal(encrypted, second) {
		t.Fatal("nonce reused")
	}
	reopened, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := reopened.Open("account-test", encrypted)
	if err != nil || !bytes.Equal(decoded, plain) {
		t.Fatal("credentials did not survive reopen", err)
	}
	if _, err := reopened.Open("other-account", encrypted); err == nil {
		t.Fatal("credentials moved to another account")
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, err := reopened.Open("account-test", encrypted); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("key permissions", err)
	}
}

func TestMissingOrCorruptKeyFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.key")
	if _, err := Open(path, false); err == nil {
		t.Fatal("missing key replaced despite existing credentials")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("missing key must not be created")
	}
	if err := os.WriteFile(path, []byte("invalid-key"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, true); err == nil {
		t.Fatal("corrupt key accepted or replaced")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "invalid-key" {
		t.Fatal("corrupt key was overwritten")
	}
}
