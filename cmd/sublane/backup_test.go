package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestMaintenanceBackupVerifyAndRestore(t *testing.T) {
	ctx := context.Background()
	source := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(source, "sublane.db"))
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
	keyFile := filepath.Join(t.TempDir(), "credentials.key")
	if _, err := vault.Open(keyFile, true); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "test.tar.gz")
	target := filepath.Join(t.TempDir(), "restored")
	for _, args := range [][]string{{"backup", "--output", archive}, {"backup", "verify", "--input", archive}, {"restore", "--input", archive, "--data-dir", target}} {
		var output bytes.Buffer
		if err := runMaintenance(ctx, append(args, "--password-stdin"), source, keyFile, strings.NewReader("synthetic backup passphrase\n"), &output); err != nil {
			t.Fatal(args[0], err)
		}
		if strings.Contains(output.String(), "synthetic backup passphrase") {
			t.Fatal("password leaked into command output")
		}
		if output.Len() == 0 {
			t.Fatal("missing command result")
		}
	}
	if _, err := os.Stat(filepath.Join(target, "sublane.db")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"backup"}, {"backup", "verify"}, {"restore", "--input", archive}, {"backup", "--input", archive}, {"restore", "--input", archive, "--data-dir", source}, {"backup", "--output", archive, "unexpected"}} {
		if err := runMaintenance(ctx, args, source, "", strings.NewReader(""), &bytes.Buffer{}); err == nil {
			t.Fatal("unsafe or invalid arguments accepted", args)
		}
	}
	if err := runMaintenance(ctx, []string{"backup", "--help"}, source, "", strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}

func TestBackupPasswordInputIsBoundedAndNeverAnArgument(t *testing.T) {
	for _, input := range []string{"", "synthetic-password\nextra", strings.Repeat("x", 515)} {
		if _, err := backupPassword(context.Background(), strings.NewReader(input)); err == nil {
			t.Fatal("invalid password input accepted")
		}
	}
	password, err := backupPassword(context.Background(), strings.NewReader("  synthetic passphrase  \r\n"))
	if err != nil || password != "  synthetic passphrase  " {
		t.Fatalf("password whitespace changed: %v", err)
	}
	if err := runMaintenance(context.Background(), []string{"backup", "--password", "synthetic-passphrase"}, t.TempDir(), "", strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("password accepted in argv")
	}
}

func TestBackupPasswordReadStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	finished := make(chan error, 1)
	signaled := &passwordReadSignal{PipeReader: reader, started: make(chan struct{})}
	go func() { _, err := backupPassword(ctx, signaled); finished <- err }()
	select {
	case <-signaled.started:
	case <-time.After(time.Second):
		t.Fatal("password read did not start")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("password read lost cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled password read remained blocked")
	}
}

type passwordReadSignal struct {
	*io.PipeReader
	started chan struct{}
}

func (reader *passwordReadSignal) Read(buffer []byte) (int, error) {
	close(reader.started)
	return reader.PipeReader.Read(buffer)
}
