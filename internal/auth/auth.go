package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInitialized = errors.New("already_initialized")
	ErrCredentials = errors.New("invalid_credentials")
	ErrInput       = errors.New("invalid_input")
	ErrBusy        = errors.New("auth_busy")
)

const SessionTTL = 12 * time.Hour

var usernamePattern = regexp.MustCompile("^[a-z0-9][a-z0-9_-]{2,31}$")

type User struct {
	Username string `json:"username"`
}
type State struct {
	Initialized bool  `json:"initialized"`
	User        *User `json:"user"`
}
type Session struct {
	Token     string
	User      User
	ExpiresAt time.Time
}
type Service struct {
	db        *sql.DB
	now       func() time.Time
	dummyHash string
	hashSlots chan struct{}
}

func New(db *sql.DB) (*Service, error) {
	s := &Service{db: db, now: time.Now, hashSlots: make(chan struct{}, 2)}
	_, err := s.Initialized(context.Background())
	if err != nil {
		return nil, err
	}
	s.dummyHash = hashPassword(newToken())
	return s, nil
}

func (s *Service) Initialized(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM administrators)").Scan(&exists)
	return exists, err
}

func (s *Service) State(ctx context.Context, token string) (State, error) {
	initialized, err := s.Initialized(ctx)
	if err != nil {
		return State{}, err
	}
	state := State{Initialized: initialized}
	if !initialized || !validToken(token) {
		return state, nil
	}
	digest := sha256.Sum256([]byte(token))
	var user User
	err = s.db.QueryRowContext(ctx, "SELECT a.username FROM admin_sessions s JOIN administrators a ON a.id = s.administrator_id WHERE s.token_hash = ? AND s.expires_at > ?", digest[:], s.now().Unix()).Scan(&user.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return State{}, err
	}
	state.User = &user
	return state, nil
}

func (s *Service) Setup(ctx context.Context, username, password string) (Session, error) {
	initialized, err := s.Initialized(ctx)
	if err != nil {
		return Session{}, err
	}
	if initialized {
		return Session{}, ErrInitialized
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if !validCredentials(username, password, 20) {
		return Session{}, ErrInput
	}
	if err := s.acquire(ctx); err != nil {
		return Session{}, err
	}
	defer func() { <-s.hashSlots }()
	hash := hashPassword(password)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()
	// The guarded insert and first session commit together; racing setup requests cannot replace the owner.
	result, err := tx.ExecContext(ctx, "INSERT INTO administrators(id, username, password_hash, created_at) SELECT 1, ?, ?, ? WHERE NOT EXISTS(SELECT 1 FROM administrators)", username, hash, s.now().Unix())
	if err != nil {
		return Session{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Session{}, err
	}
	if n == 0 {
		return Session{}, ErrInitialized
	}
	session, err := s.createSession(ctx, tx, username)
	if err != nil {
		return Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (Session, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	// Keep existing accounts usable if their passwords predate the 20-character setup limit.
	if !validCredentials(username, password, 128) {
		return Session{}, ErrCredentials
	}
	if err := s.acquire(ctx); err != nil {
		return Session{}, err
	}
	defer func() { <-s.hashSlots }()
	var hash, storedName string
	err := s.db.QueryRowContext(ctx, "SELECT username, password_hash FROM administrators WHERE username = ?", username).Scan(&storedName, &hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Session{}, err
	}
	found := err == nil
	if !found {
		hash = s.dummyHash
	}
	// Unknown usernames still perform the same KDF and return the same public error.
	matches := verifyPassword(hash, password)
	if !found || !matches {
		return Session{}, ErrCredentials
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()
	session, err := s.createSession(ctx, tx, storedName)
	if err != nil {
		return Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) createSession(ctx context.Context, tx *sql.Tx, username string) (Session, error) {
	now := s.now()
	session := Session{Token: newToken(), User: User{Username: username}, ExpiresAt: now.Add(SessionTTL)}
	digest := sha256.Sum256([]byte(session.Token))
	if _, err := tx.ExecContext(ctx, "DELETE FROM admin_sessions WHERE expires_at <= ?", now.Unix()); err != nil {
		return Session{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO admin_sessions(token_hash, administrator_id, created_at, expires_at) VALUES (?, 1, ?, ?)", digest[:], now.Unix(), session.ExpiresAt.Unix()); err != nil {
		return Session{}, err
	}
	// Preserve the new token even when multiple sessions share the same timestamp.
	if _, err := tx.ExecContext(ctx, "DELETE FROM admin_sessions WHERE token_hash IN (SELECT token_hash FROM admin_sessions WHERE token_hash != ? ORDER BY created_at DESC, rowid DESC LIMIT -1 OFFSET 4)", digest[:]); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) Revoke(ctx context.Context, token string) error {
	digest := sha256.Sum256([]byte(token))
	_, err := s.db.ExecContext(ctx, "DELETE FROM admin_sessions WHERE token_hash = ?", digest[:])
	return err
}

func (s *Service) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case s.hashSlots <- struct{}{}:
		return nil
	default:
		return ErrBusy
	}
}

func validCredentials(username, password string, maxLength int) bool {
	n := utf8.RuneCountInString(password)
	return usernamePattern.MatchString(username) && utf8.ValidString(password) && n >= 8 && n <= maxLength
}

func newToken() string {
	data := make([]byte, 32)
	_, _ = rand.Read(data)
	return base64.RawURLEncoding.EncodeToString(data)
}

func validToken(token string) bool {
	if len(token) != 43 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(raw) == 32
}
