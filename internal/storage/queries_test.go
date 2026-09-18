package storage_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/storage/db"
)

func TestGeneratedQueriesKeepTransactionsOnTheirConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := storage.Open(ctx, filepath.Join(t.TempDir(), "queries.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	queries := db.New(connection)
	for _, commit := range []bool{false, true} {
		tx, err := connection.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		q := queries.WithTx(tx)
		count, err := q.CreateAdministrator(ctx, db.CreateAdministratorParams{Username: "owner-test", PasswordHash: "synthetic-hash", CreatedAt: 100})
		if err != nil || count != 1 {
			t.Fatalf("insert owner: count=%d err=%v", count, err)
		}
		digest := sha256.Sum256([]byte("synthetic-session"))
		if err := q.CreateSession(ctx, db.CreateSessionParams{TokenHash: digest[:], UserID: 1, CreatedAt: 100, ExpiresAt: 200}); err != nil {
			t.Fatal(err)
		}
		user, err := q.GetSessionUser(ctx, db.GetSessionUserParams{TokenHash: digest[:], Now: 150})
		if err != nil || user.Username != "owner-test" || user.Role != "admin" {
			t.Fatalf("transaction cannot read its writes: %v", err)
		}
		if commit {
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
		} else if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		_, err = queries.GetSessionUser(ctx, db.GetSessionUserParams{TokenHash: digest[:], Now: 150})
		if commit && err != nil || !commit && !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("commit=%v persisted session error=%v", commit, err)
		}
		exists, err := queries.HasAdministrator(ctx)
		if err != nil || exists != commit {
			t.Fatalf("commit=%v persisted owner=%v err=%v", commit, exists, err)
		}
	}
}
