package auth

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage"
)

const syntheticPassword = "fake password 42"

func fixture(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(context.Background(), filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	return s, db
}

func TestSetupPasswordLength(t *testing.T) {
	for _, tc := range []struct {
		name     string
		password string
		valid    bool
	}{
		{"below minimum", strings.Repeat("x", 7), false},
		{"minimum", strings.Repeat("x", 8), true},
		{"maximum", strings.Repeat("x", 20), true},
		{"above maximum", strings.Repeat("x", 21), false},
		{"short Unicode", strings.Repeat("🙂", 7), false},
		{"Unicode maximum", strings.Repeat("🙂", 20), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := fixture(t)
			ctx := context.Background()
			_, err := s.Setup(ctx, "admin-test", tc.password, "Synthetic workspace")
			if !tc.valid {
				if !errors.Is(err, ErrInput) {
					t.Fatalf("invalid password accepted: %v", err)
				}
				initialized, err := s.Initialized(ctx)
				if err != nil || initialized {
					t.Fatalf("invalid setup created an administrator: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("valid password rejected: %v", err)
			}
			if _, err := s.Login(ctx, "admin-test", tc.password); err != nil {
				t.Fatalf("cannot log in with accepted password: %v", err)
			}
		})
	}
}

func TestLoginPreservesLegacyPasswordLengths(t *testing.T) {
	for _, length := range []int{21, 128} {
		s, db := fixture(t)
		password := strings.Repeat("x", length)
		if _, err := db.Exec("INSERT INTO users(id, username, role, password_hash, created_at) VALUES(1, ?, 'admin', ?, ?)", "admin-test", hashPassword(password), s.now().Unix()); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Login(context.Background(), "admin-test", password); err != nil {
			t.Fatalf("legacy password of length %d rejected: %v", length, err)
		}
		if _, err := s.Login(context.Background(), "admin-test", strings.Repeat("x", 129)); !errors.Is(err, ErrCredentials) {
			t.Fatalf("oversized login password accepted: %v", err)
		}
	}
}

func TestSetupSessionAndRevocation(t *testing.T) {
	s, db := fixture(t)
	ctx := context.Background()
	state, err := s.State(ctx, "")
	if err != nil || state.Initialized || state.User != nil {
		t.Fatalf("initial state: %+v, %v", state, err)
	}
	if _, err := s.Setup(ctx, "admin-test", "short", "Synthetic workspace"); !errors.Is(err, ErrInput) {
		t.Fatalf("short password accepted: %v", err)
	}
	session, err := s.Setup(ctx, "Admin-Test", syntheticPassword, "Synthetic workspace")
	if err != nil {
		t.Fatal(err)
	}
	var ownerID int64
	var role string
	if err := db.QueryRow(`SELECT t.owner_user_id,m.role FROM tenants t
		JOIN memberships m ON m.tenant_id=t.id AND m.user_id=t.owner_user_id
		WHERE t.id=1`).Scan(&ownerID, &role); err != nil || ownerID != session.User.ID || role != "owner" {
		t.Fatalf("setup did not create the initial workspace owner: id=%d role=%q err=%v", ownerID, role, err)
	}
	if session.User.Username != "admin-test" {
		t.Fatal("username is not normalized")
	}
	if _, err := s.Setup(ctx, "another-test", syntheticPassword, "Synthetic workspace"); !errors.Is(err, ErrInitialized) {
		t.Fatalf("setup reopened: %v", err)
	}
	var hash string
	var digest []byte
	if err := db.QueryRow("SELECT password_hash FROM users").Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") || strings.Contains(hash, syntheticPassword) {
		t.Fatal("password is not hashed")
	}
	if err := db.QueryRow("SELECT token_hash FROM sessions").Scan(&digest); err != nil {
		t.Fatal(err)
	}
	if len(digest) != 32 || string(digest) == session.Token {
		t.Fatal("session token must be stored as a digest")
	}
	restarted, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Setup(ctx, "another-test", syntheticPassword, "Synthetic workspace"); !errors.Is(err, ErrInitialized) {
		t.Fatalf("restart reopened setup: %v", err)
	}
	state, err = restarted.State(ctx, session.Token)
	if err != nil || state.User == nil || state.User.Username != "admin-test" {
		t.Fatalf("session did not survive restart: %+v %v", state, err)
	}
	if err := restarted.Revoke(ctx, session.Token); err != nil {
		t.Fatal(err)
	}
	state, err = s.State(ctx, session.Token)
	if err != nil || state.User != nil {
		t.Fatal("revoked session still works")
	}
}

func TestConcurrentSetupCreatesExactlyOneAdministrator(t *testing.T) {
	s, db := fixture(t)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Setup(context.Background(), "admin-test", syntheticPassword, "Synthetic workspace")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, closed := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrInitialized):
			closed++
		default:
			t.Fatal(err)
		}
	}
	var count int
	_ = db.QueryRow("SELECT count(*) FROM users").Scan(&count)
	if success != 1 || closed != 1 || count != 1 {
		t.Fatalf("setup race: success=%d closed=%d count=%d", success, closed, count)
	}
}

func TestLoginExpiryAndSessionBound(t *testing.T) {
	s, db := fixture(t)
	ctx := context.Background()
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	first, err := s.Setup(ctx, "admin-test", syntheticPassword, "Synthetic workspace")
	if err != nil {
		t.Fatal(err)
	}
	for _, credentials := range [][2]string{{"unknown-test", syntheticPassword}, {"admin-test", "wrong synthetic passphrase"}} {
		if _, err := s.Login(ctx, credentials[0], credentials[1]); !errors.Is(err, ErrCredentials) {
			t.Fatalf("unexpected login result: %v", err)
		}
	}
	var last Session
	for range 6 {
		now = now.Add(time.Second)
		last, err = s.Login(ctx, "ADMIN-TEST", syntheticPassword)
		if err != nil {
			t.Fatal(err)
		}
		if last.Token == first.Token {
			t.Fatal("login reused a token")
		}
	}
	var count int
	_ = db.QueryRow("SELECT count(*) FROM sessions").Scan(&count)
	if count != 5 {
		t.Fatalf("unbounded sessions: %d", count)
	}
	state, err := s.State(ctx, first.Token)
	if err != nil || state.User != nil {
		t.Fatal("oldest session was not evicted")
	}
	now = last.ExpiresAt
	state, err = s.State(ctx, last.Token)
	if err != nil || state.User != nil {
		t.Fatal("expired session still authenticates")
	}
}

func TestInputAndHashWorkAreBounded(t *testing.T) {
	s, _ := fixture(t)
	for _, name := range []string{"x", "spaces are invalid", strings.Repeat("x", 33)} {
		if _, err := s.Setup(context.Background(), name, syntheticPassword, "Synthetic workspace"); !errors.Is(err, ErrInput) {
			t.Fatalf("invalid username accepted: %q", name)
		}
	}
	if _, err := s.Setup(context.Background(), "admin-test", strings.Repeat("界", 129), "Synthetic workspace"); !errors.Is(err, ErrInput) {
		t.Fatal("oversized Unicode password accepted")
	}
	if _, err := s.Setup(context.Background(), "admin-test", syntheticPassword, "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	s.hashSlots <- struct{}{}
	s.hashSlots <- struct{}{}
	if _, err := s.Login(context.Background(), "admin-test", syntheticPassword); !errors.Is(err, ErrBusy) {
		t.Fatalf("hash work is not bounded: %v", err)
	}
	<-s.hashSlots
	<-s.hashSlots
}

func TestFailedSetupRollsBackAndCanRetry(t *testing.T) {
	s, db := fixture(t)
	var schema string
	if err := db.QueryRow("SELECT sql FROM sqlite_master WHERE name = 'sessions'").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TABLE sessions"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Setup(context.Background(), "admin-test", syntheticPassword, "Synthetic workspace"); err == nil {
		t.Fatal("setup succeeded without session persistence")
	}
	initialized, err := s.Initialized(context.Background())
	if err != nil || initialized {
		t.Fatal("failed setup left a partially created administrator")
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Setup(context.Background(), "admin-test", syntheticPassword, "Synthetic workspace"); err != nil {
		t.Fatalf("setup cannot be retried after rollback: %v", err)
	}
}
