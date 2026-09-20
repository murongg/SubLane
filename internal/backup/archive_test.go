package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

type testEntry struct {
	name string
	data []byte
	kind byte
}

func archiveFixture(t *testing.T) (string, []testEntry) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, databaseName))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	identity, err := auth.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "synthetic-admin", "synthetic-pass"); err != nil {
		t.Fatal(err)
	}
	cipher, err := vault.Open(filepath.Join(dir, keyName), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := apikey.New(connection, cipher).Create(ctx, 1, "Synthetic backup key"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "backup.tar.gz")
	if _, err := Create(ctx, dir, path, "synthetic-version"); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	var entries []testEntry
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, testEntry{header.Name, data, header.Typeflag})
	}
	return path, entries
}
func encodeArchive(t *testing.T, entries []testEntry) []byte {
	t.Helper()
	var data bytes.Buffer
	compressed := gzip.NewWriter(&data)
	archive := tar.NewWriter(compressed)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: 0600, Size: int64(len(entry.data)), Typeflag: entry.kind, Format: tar.FormatUSTAR}
		if entry.kind == tar.TypeSymlink {
			header.Linkname = "/synthetic/outside"
			header.Size = 0
		}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := archive.Write(entry.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func TestArchiveRejectsCorruptionUnsafeEntriesAndWrongKey(t *testing.T) {
	path, original := archiveFixture(t)
	for _, name := range []string{"missing-key", "duplicate", "path", "symlink", "digest", "wrong-key", "truncated", "footer", "appended", "future-schema"} {
		t.Run(name, func(t *testing.T) {
			entries := make([]testEntry, len(original))
			for i, e := range original {
				entries[i] = testEntry{e.name, append([]byte(nil), e.data...), e.kind}
			}
			switch name {
			case "missing-key":
				entries = entries[:2]
			case "duplicate":
				entries = append(entries, entries[1])
			case "path":
				entries[2].name = "../credentials.key"
			case "symlink":
				entries[2].kind = tar.TypeSymlink
			case "digest", "wrong-key":
				entries[2].data = bytes.Repeat([]byte{7}, 32)
			case "future-schema":
				dbPath := filepath.Join(t.TempDir(), databaseName)
				if err := os.WriteFile(dbPath, entries[1].data, 0600); err != nil {
					t.Fatal(err)
				}
				connection, err := storage.Open(context.Background(), dbPath)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := connection.Exec("INSERT INTO schema_migrations(name) VALUES('999_future.sql')"); err != nil {
					t.Fatal(err)
				}
				if err := connection.Close(); err != nil {
					t.Fatal(err)
				}
				entries[1].data, err = os.ReadFile(dbPath)
				if err != nil {
					t.Fatal(err)
				}
			}
			if name == "wrong-key" || name == "future-schema" {
				var value manifest
				if err := json.Unmarshal(entries[0].data, &value); err != nil {
					t.Fatal(err)
				}
				for _, e := range entries[1:] {
					sum := sha256.Sum256(e.data)
					value.Files[e.name] = digest{int64(len(e.data)), hex.EncodeToString(sum[:])}
				}
				value.DatabaseBytes = int64(len(entries[1].data))
				if name == "future-schema" {
					value.SchemaVersion++
				}
				entries[0].data, _ = json.Marshal(value)
			}
			raw := encodeArchive(t, entries)
			switch name {
			case "truncated":
				raw = raw[:len(raw)/2]
			case "footer":
				raw[len(raw)-4] ^= 1
			case "appended":
				raw = append(raw, raw...)
			}
			bad := filepath.Join(t.TempDir(), "bad.tar.gz")
			if err := os.WriteFile(bad, raw, 0600); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "restored")
			if _, err := Restore(context.Background(), bad, target); err == nil {
				t.Fatal("invalid archive accepted")
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("failed restore published data", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Verify(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
func TestRestoreDoesNotReplaceExistingEmptyDirectory(t *testing.T) {
	path, _ := archiveFixture(t)
	target := t.TempDir()
	if _, err := Restore(context.Background(), path, target); !errors.Is(err, ErrExists) {
		t.Fatal("existing directory accepted", err)
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatal("existing directory changed", err)
	}
	source := filepath.Join(t.TempDir(), "staged")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := publish(source, target); !errors.Is(err, ErrExists) {
		t.Fatal("publication overwrote a concurrently created directory", err)
	}
}

func TestRestoreMigratesKnownOlderSchemaOnlyInStaging(t *testing.T) {
	_, entries := archiveFixture(t)
	path := filepath.Join(t.TempDir(), databaseName)
	if err := os.WriteFile(path, entries[1].data, 0600); err != nil {
		t.Fatal(err)
	}
	connection, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec("DROP INDEX request_records_request_id; ALTER TABLE request_records DROP COLUMN request_id; ALTER TABLE request_records DROP COLUMN first_token_ms; ALTER TABLE account_usage DROP COLUMN revision; DELETE FROM schema_migrations WHERE name='019_request_diagnostics.sql'"); err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	entries[1].data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value manifest
	if err := json.Unmarshal(entries[0].data, &value); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(entries[1].data)
	value.SchemaVersion = 18
	value.DatabaseBytes = int64(len(entries[1].data))
	value.Files[databaseName] = digest{value.DatabaseBytes, hex.EncodeToString(sum[:])}
	entries[0].data, _ = json.Marshal(value)
	raw := encodeArchive(t, entries)
	archive := filepath.Join(t.TempDir(), "older.tar.gz")
	if err := os.WriteFile(archive, raw, 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "restored")
	if _, err := Restore(context.Background(), archive, target); err != nil {
		t.Fatal(err)
	}
	restored, err := storage.OpenReadOnly(context.Background(), filepath.Join(target, databaseName))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if schema, err := storage.ValidateSnapshot(context.Background(), restored); err != nil || schema != 19 {
		t.Fatal("older backup not migrated", schema, err)
	}
	actual, err := os.ReadFile(archive)
	if err != nil || !bytes.Equal(actual, raw) {
		t.Fatal("restore changed its input archive", err)
	}
}
func TestArchiveRejectsOversizedDeclaredEntry(t *testing.T) {
	var data bytes.Buffer
	compressed := gzip.NewWriter(&data)
	archive := tar.NewWriter(compressed)
	if err := archive.WriteHeader(&tar.Header{Name: databaseName, Mode: 0600, Size: MaxDatabaseBytes + 1, Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}); err != nil {
		t.Fatal(err)
	}
	_ = archive.Close()
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "oversized.tar.gz")
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(context.Background(), path); err == nil {
		t.Fatal("oversized header accepted")
	}
}

func TestBackupRejectsOversizedDatabaseRecords(t *testing.T) {
	ctx := context.Background()
	source := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(source, databaseName))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	cipher, err := vault.Open(filepath.Join(source, keyName), true)
	if err != nil {
		t.Fatal(err)
	}
	account, err := accounts.New(connection, cipher).Authorize(ctx, "Synthetic account", accounts.Credential{AccountID: "synthetic-subject", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	// Embedded NUL bytes must not defeat byte-length limits on SQLite TEXT values.
	if _, err := connection.Exec("UPDATE accounts SET name=CAST(zeroblob(1048577) AS TEXT) WHERE id=?", account.ID); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "bounded.tar.gz")
	if _, err := Create(ctx, source, output, "synthetic-version"); err == nil {
		t.Fatal("oversized account metadata was accepted")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("oversized backup was published", err)
	}
}
