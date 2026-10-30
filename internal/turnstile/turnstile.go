// Package turnstile owns instance-wide login verification and its encrypted configuration.
package turnstile

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
)

var (
	ErrInput       = errors.New("invalid_turnstile_settings")
	ErrRejected    = errors.New("turnstile_failed")
	ErrUnavailable = errors.New("turnstile_unavailable")
)

type State struct {
	Enabled          bool   `json:"enabled"`
	SiteKey          string `json:"site_key"`
	SecretConfigured bool   `json:"secret_configured"`
}
type Input struct {
	Enabled   bool    `json:"enabled"`
	SiteKey   string  `json:"site_key"`
	SecretKey *string `json:"secret_key,omitempty"`
}
type config struct {
	Enabled   bool   `json:"enabled"`
	SiteKey   string `json:"site_key"`
	SecretKey string `json:"secret_key"`
}
type Service struct {
	conn    *sql.DB
	vault   *vault.Vault
	client  *http.Client
	mu      sync.Mutex
	current atomic.Pointer[config]
}

func New(ctx context.Context, conn *sql.DB, cipher *vault.Vault, client *http.Client) (*Service, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	s := &Service{conn: conn, vault: cipher, client: client}
	c := &config{}
	stored, err := db.New(conn).GetTurnstileSetting(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		if len(stored) > 4096 {
			return nil, ErrInput
		}
		encrypted, err := base64.StdEncoding.DecodeString(stored)
		if err != nil {
			return nil, ErrInput
		}
		plain, err := cipher.OpenSetting("turnstile", encrypted)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(plain, c); err != nil {
			return nil, ErrInput
		}
		if !valid(*c) {
			return nil, ErrInput
		}
	}
	s.current.Store(c)
	return s, nil
}

func validKey(key string) bool {
	if len(key) > 256 {
		return false
	}
	for _, c := range key {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func valid(c config) bool {
	return validKey(c.SiteKey) && validKey(c.SecretKey) && (!c.Enabled || c.SiteKey != "" && c.SecretKey != "")
}
func (s *Service) State() State {
	c := s.current.Load()
	return State{Enabled: c.Enabled, SiteKey: c.SiteKey, SecretConfigured: c.SecretKey != ""}
}
func (s *Service) SiteKey() string {
	c := s.current.Load()
	if !c.Enabled {
		return ""
	}
	return c.SiteKey
}
func (s *Service) Save(ctx context.Context, input Input) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *s.current.Load()
	c.Enabled, c.SiteKey = input.Enabled, input.SiteKey
	if input.SecretKey != nil {
		c.SecretKey = *input.SecretKey
	}
	if !valid(c) {
		return State{}, ErrInput
	}
	plain, err := json.Marshal(c)
	if err != nil {
		return State{}, err
	}
	encrypted, err := s.vault.SealSetting("turnstile", plain)
	if err != nil {
		return State{}, err
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	q := db.New(s.conn).WithTx(tx)
	if err := q.SaveTurnstileSetting(ctx, base64.StdEncoding.EncodeToString(encrypted)); err != nil {
		return State{}, err
	}
	if err := audit.Record(ctx, q, "settings.update", "settings", "turnstile"); err != nil {
		return State{}, err
	}
	if err := tx.Commit(); err != nil {
		return State{}, err
	}
	// Publish the complete configuration only after both the setting and audit event are durable.
	s.current.Store(&c)
	return s.State(), nil
}

func (s *Service) Verify(ctx context.Context, token, hostname string) error {
	c := s.current.Load()
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(token) == "" || len(token) > 2048 {
		return ErrRejected
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	form := url.Values{"secret": {c.SecretKey}, "response": {token}}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return ErrUnavailable
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.client.Do(r)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil || len(body) > 16384 {
		return ErrUnavailable
	}
	var result struct {
		Success  bool   `json:"success"`
		Hostname string `json:"hostname"`
		Action   string `json:"action"`
	}
	if json.Unmarshal(body, &result) != nil {
		return ErrUnavailable
	}
	// Tokens from another host or another form must never authorize this login.
	if !result.Success || result.Hostname != hostname || result.Action != "login" {
		return ErrRejected
	}
	return nil
}
