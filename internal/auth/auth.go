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

	"github.com/murongg/SubLane/internal/storage/db"
)

var (
	ErrInitialized = errors.New("already_initialized")
	ErrCredentials = errors.New("invalid_credentials")
	ErrInput       = errors.New("invalid_input")
	ErrBusy        = errors.New("auth_busy")
)

const SessionTTL = 12 * time.Hour

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

var usernamePattern = regexp.MustCompile("^[a-z0-9][a-z0-9_-]{2,31}$")

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     Role   `json:"role"`
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
	queries   *db.Queries
	now       func() time.Time
	dummyHash string
	hashSlots chan struct{}
}

func New(connection *sql.DB) (*Service, error) {
	s := &Service{db: connection, queries: db.New(connection), now: time.Now, hashSlots: make(chan struct{}, 2)}
	_, err := s.Initialized(context.Background())
	if err != nil {
		return nil, err
	}
	s.dummyHash = hashPassword(newToken())
	return s, nil
}

func (s *Service) Initialized(ctx context.Context) (bool, error) {
	return s.queries.HasAdministrator(ctx)
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
	user, err := s.queries.GetSessionUser(ctx, db.GetSessionUserParams{TokenHash: digest[:], Now: s.now().Unix()})
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return State{}, err
	}
	state.User = &User{ID: user.ID, Username: user.Username, Role: Role(user.Role)}
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
	n, err := s.queries.WithTx(tx).CreateAdministrator(ctx, db.CreateAdministratorParams{Username: username, PasswordHash: hash, CreatedAt: s.now().Unix()})
	if err != nil {
		return Session{}, err
	}
	if n == 0 {
		return Session{}, ErrInitialized
	}
	session, err := s.createSession(ctx, tx, User{ID: 1})
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
	user, err := s.queries.GetLoginUser(ctx, username)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Session{}, err
	}
	found := err == nil
	hash := user.PasswordHash
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
	session, err := s.createSession(ctx, tx, User{ID: user.ID})
	if err != nil {
		return Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) createSession(ctx context.Context, tx *sql.Tx, user User) (Session, error) {
	queries := s.queries.WithTx(tx)
	// Recheck inside the transaction: a member may be disabled while password hashing is in progress.
	stored, err := queries.GetEnabledUser(ctx, user.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrCredentials
	}
	if err != nil {
		return Session{}, err
	}
	user = User{ID: stored.ID, Username: stored.Username, Role: Role(stored.Role)}
	now := s.now()
	session := Session{Token: newToken(), User: user, ExpiresAt: now.Add(SessionTTL)}
	digest := sha256.Sum256([]byte(session.Token))
	if err := queries.DeleteExpiredSessions(ctx, now.Unix()); err != nil {
		return Session{}, err
	}
	if err := queries.CreateSession(ctx, db.CreateSessionParams{TokenHash: digest[:], UserID: user.ID, CreatedAt: now.Unix(), ExpiresAt: session.ExpiresAt.Unix()}); err != nil {
		return Session{}, err
	}
	// Preserve the new token even when multiple sessions share the same timestamp.
	if err := queries.TrimSessions(ctx, db.TrimSessionsParams{UserID: user.ID, CurrentTokenHash: digest[:]}); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) Revoke(ctx context.Context, token string) error {
	digest := sha256.Sum256([]byte(token))
	return s.queries.RevokeSession(ctx, digest[:])
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
