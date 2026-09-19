package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage/db"
)

var (
	ErrInput            = errors.New("invalid_input")
	ErrLimit            = errors.New("api_key_limit")
	ErrNotFound         = errors.New("api_key_not_found")
	ErrInvalidKey       = errors.New("invalid_api_key")
	ErrOwnerUnavailable = errors.New("unauthorized")
)

const maxActiveKeys = 20

type Key struct {
	ID          int64  `json:"id"`
	GroupID     int64  `json:"group_id"`
	GroupName   string `json:"group_name"`
	GroupAccess string `json:"group_access"`
	Name        string `json:"name"`
	Prefix      string `json:"prefix"`
	CreatedAt   int64  `json:"created_at"`
	LastUsedAt  *int64 `json:"last_used_at"`
	RevokedAt   *int64 `json:"revoked_at"`
}
type CreatedKey struct {
	Key    Key    `json:"key"`
	Secret string `json:"secret"`
}
type Page struct {
	Keys       []Key `json:"keys"`
	NextCursor int64 `json:"next_cursor"`
}
type Principal struct{ KeyID, UserID, GroupID int64 }
type Service struct {
	db      *sql.DB
	queries *db.Queries
	now     func() time.Time
}

func New(connection *sql.DB) *Service {
	return &Service{db: connection, queries: db.New(connection), now: time.Now}
}

func (s *Service) Create(ctx context.Context, userID int64, name string) (CreatedKey, error) {
	return s.CreateInGroup(ctx, userID, groups.DefaultID, name)
}

func (s *Service) CreateInGroup(ctx context.Context, userID, groupID int64, name string) (CreatedKey, error) {
	if groupID <= 0 {
		return CreatedKey{}, ErrInput
	}
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return CreatedKey{}, ErrInput
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	secret := "sl_" + base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(secret))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CreatedKey{}, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	enabled, err := queries.GetKeyOwnerEnabled(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !enabled) {
		return CreatedKey{}, ErrOwnerUnavailable
	}
	if err != nil {
		return CreatedKey{}, err
	}
	allowed, err := queries.CanUseGroup(ctx, db.CanUseGroupParams{UserID: userID, GroupID: groupID})
	if err != nil {
		return CreatedKey{}, err
	}
	if !allowed {
		return CreatedKey{}, groups.ErrUnavailable
	}
	group, err := queries.GetGroup(ctx, groupID)
	if err != nil {
		return CreatedKey{}, err
	}
	count, err := queries.CountActiveKeys(ctx, userID)
	if err != nil {
		return CreatedKey{}, err
	}
	if count >= maxActiveKeys {
		return CreatedKey{}, ErrLimit
	}
	key := Key{GroupID: groupID, GroupName: group.Name, GroupAccess: "allowed", Name: name, Prefix: secret[:11], CreatedAt: s.now().Unix()}
	key.ID, err = queries.CreateKey(ctx, db.CreateKeyParams{GroupID: groupID, UserID: userID, Name: name, Prefix: key.Prefix, TokenHash: digest[:], CreatedAt: key.CreatedAt})
	if err != nil {
		return CreatedKey{}, err
	}
	if err := tx.Commit(); err != nil {
		return CreatedKey{}, err
	}
	return CreatedKey{Key: key, Secret: secret}, nil
}

func (s *Service) List(ctx context.Context, userID, beforeID int64) (Page, error) {
	page := Page{Keys: []Key{}}
	if beforeID < 0 {
		return page, ErrInput
	}
	rows, err := s.queries.ListKeys(ctx, db.ListKeysParams{UserID: userID, BeforeID: beforeID})
	if err != nil {
		return page, err
	}
	for _, row := range rows {
		page.Keys = append(page.Keys, Key(row))
	}
	if len(page.Keys) > 50 {
		page.Keys = page.Keys[:50]
		page.NextCursor = page.Keys[49].ID
	}
	return page, nil
}

func (s *Service) Revoke(ctx context.Context, userID, keyID int64) (Key, error) {
	now := s.now().Unix()

	n, err := s.queries.RevokeKey(ctx, db.RevokeKeyParams{Now: &now, ID: keyID, UserID: userID})
	if err != nil {
		return Key{}, err
	}
	if n == 0 {
		return Key{}, ErrNotFound
	}
	row, err := s.queries.GetKey(ctx, db.GetKeyParams{ID: keyID, UserID: userID})
	return Key(row), err
}

func (s *Service) Authenticate(ctx context.Context, secret string) (Principal, error) {
	if len(secret) != 46 || !strings.HasPrefix(secret, "sl_") {
		return Principal{}, ErrInvalidKey
	}
	raw, err := base64.RawURLEncoding.DecodeString(secret[3:])
	if err != nil || len(raw) != 32 {
		return Principal{}, ErrInvalidKey
	}
	digest := sha256.Sum256([]byte(secret))
	// Recheck membership and pool enablement on every request, including later WebSocket turns.
	row, err := s.queries.AuthenticateKey(ctx, digest[:])
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrInvalidKey
	}
	if err != nil {
		return Principal{}, err
	}
	now := s.now().Unix()
	threshold := now - 60
	// Track usage without writing to SQLite for every request in a burst.
	err = s.queries.TouchKey(ctx, db.TouchKeyParams{Now: &now, ID: row.ID, Threshold: &threshold})
	return Principal{KeyID: row.ID, UserID: row.UserID, GroupID: row.GroupID}, err
}
