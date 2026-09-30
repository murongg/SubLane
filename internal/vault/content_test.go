package vault

import (
	"path/filepath"
	"testing"
)

func TestContentBinding(t *testing.T) {
	v, err := Open(filepath.Join(t.TempDir(), "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := v.SealContent(1, []byte("synthetic-rule"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = v.OpenContent(2, encrypted); err == nil {
		t.Fatal("cross-workspace decrypt accepted")
	}
	if _, err = v.Open("1", encrypted); err == nil {
		t.Fatal("cross-domain decrypt accepted")
	}
	plain, err := v.OpenContent(1, encrypted)
	if err != nil || string(plain) != "synthetic-rule" {
		t.Fatal("roundtrip failed", err)
	}
}
