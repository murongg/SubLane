package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeyPathSeparationAndCompatibility(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	if err := os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	legacy, err := KeyPath(data, "")
	if err != nil || legacy != filepath.Join(data, "credentials.key") {
		t.Fatalf("legacy path: %q %v", legacy, err)
	}
	outside := filepath.Join(root, "secrets", "nested", "credentials.key")
	if _, err := KeyPath(data, outside); err != nil {
		t.Fatal(err)
	}
	if _, err := KeyPath(data, outside+string(filepath.Separator)); err == nil {
		t.Fatal("directory path accepted as a key file")
	}
	for _, path := range []string{data, filepath.Join(data, "credentials.key"), filepath.Join(data, "nested", "credentials.key")} {
		if _, err := KeyPath(data, path); err == nil {
			t.Fatalf("key stayed within data directory: %q", path)
		}
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(data, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := KeyPath(data, filepath.Join(alias, "nested", "credentials.key")); err == nil {
		t.Fatal("symlink parent bypassed key separation")
	}
	caseAlias := filepath.Join(root, "DATA")
	if _, err := os.Stat(caseAlias); err == nil {
		if _, err := KeyPath(data, filepath.Join(caseAlias, "nested", "credentials.key")); err == nil {
			t.Fatal("case-insensitive directory alias bypassed key separation")
		}
	}
}
