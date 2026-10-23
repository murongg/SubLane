package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestScheduledBackupVerifiesBeforeRotating(t *testing.T) {
	ctx := context.Background()
	source, destination := t.TempDir(), t.TempDir()
	if err := os.Chmod(destination, 0700); err != nil {
		t.Fatal(err)
	}
	conn, err := storage.Open(ctx, filepath.Join(source, databaseName))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := vault.Open(filepath.Join(source, keyName), true); err != nil {
		t.Fatal(err)
	}
	passwordFile := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(passwordFile, []byte(syntheticBackupPassword+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	schedule := Schedule{Directory: destination, PasswordFile: passwordFile, Interval: 24 * time.Hour, Keep: 1}
	old := filepath.Join(destination, "scheduled-20000101T000000.000000000Z.sublane-backup.tar.gz.age")
	manual := filepath.Join(destination, "manual.sublane-backup.tar.gz.age")
	for _, path := range []string{old, manual} {
		if err := os.WriteFile(path, []byte("synthetic archive"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path, err := schedule.Run(ctx, source, "synthetic-version", "")
	if err != nil {
		t.Fatal(err)
	}
	if info, err := Verify(ctx, path, syntheticBackupPassword); err != nil || !info.Encrypted {
		t.Fatal(info, err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old scheduled backup not rotated", err)
	}
	if _, err := os.Stat(manual); err != nil {
		t.Fatal("manual archive changed", err)
	}
	if err := os.WriteFile(passwordFile, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := schedule.Run(ctx, source, "synthetic-version", ""); err == nil {
		t.Fatal("invalid password accepted")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("failed backup removed last good archive", err)
	}
}

func TestScheduleRejectsUnsafeConfiguration(t *testing.T) {
	for _, schedule := range []Schedule{
		{Directory: "backups", Interval: time.Hour, Keep: 7},
		{Directory: "backups", PasswordFile: "password", Interval: time.Second, Keep: 7},
		{Directory: "backups", PasswordFile: "password", Interval: time.Hour, Keep: 0},
	} {
		if err := schedule.Validate(); err == nil {
			t.Fatal("invalid schedule accepted")
		}
	}
	dir := t.TempDir()
	password := filepath.Join(dir, "password")
	if err := os.WriteFile(password, []byte(syntheticBackupPassword), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(password, 0644); err != nil {
		t.Fatal(err)
	}
	schedule := Schedule{Directory: t.TempDir(), PasswordFile: password, Interval: time.Hour, Keep: 7}
	if _, err := schedule.Run(context.Background(), dir, "synthetic", ""); err == nil {
		t.Fatal("public password file accepted")
	}
	if err := os.Chmod(password, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "password-link")
	if err := os.Symlink(password, link); err != nil {
		t.Fatal(err)
	}
	schedule.PasswordFile = link
	if _, err := schedule.Run(context.Background(), dir, "synthetic", ""); err == nil {
		t.Fatal("password symlink accepted")
	}
}
