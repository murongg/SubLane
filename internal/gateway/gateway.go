package gateway

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/codex"
	"github.com/murongg/SubLane/internal/storage/db"
)

var (
	ErrNoAccount     = errors.New("no_accounts_available")
	ErrBusy          = errors.New("gateway_busy")
	ErrAffinityLimit = errors.New("conversation_limit")
)

type Kind uint8

const (
	Responses Kind = iota
	Chat
	Compact
)

type Service struct {
	db       *sql.DB
	queries  *db.Queries
	accounts *accounts.Service
	provider *codex.Client
	slots    chan struct{}
	mu       sync.Mutex
	next     int
}

func New(connection *sql.DB, accounts *accounts.Service, provider *codex.Client) *Service {
	return &Service{db: connection, queries: db.New(connection), accounts: accounts, provider: provider, slots: make(chan struct{}, 8)}
}

func (s *Service) Acquire() (func(), error) {
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, nil
	default:
		return nil, ErrBusy
	}
}

func (s *Service) Open(ctx context.Context, userID int64, raw []byte, headers http.Header, kind Kind) (*codex.Stream, error) {
	var input map[string]json.RawMessage
	if json.Unmarshal(raw, &input) != nil || input == nil {
		return nil, codex.ErrInput
	}
	session := headers.Get("Session_id")
	if session == "" && len(input["prompt_cache_key"]) > 0 {
		if json.Unmarshal(input["prompt_cache_key"], &session) != nil {
			return nil, codex.ErrInput
		}
	}
	if len(session) > 1024 {
		return nil, codex.ErrInput
	}
	id, digest, err := s.selectAccount(ctx, userID, session)
	if err != nil {
		return nil, err
	}
	outgoing := headers.Clone()
	if outgoing == nil {
		outgoing = make(http.Header)
	}
	// Isolate provider-side session and prompt-cache identifiers between SubLane members.
	outgoing.Set("Session_id", fmt.Sprintf("%x-%x-%x-%x-%x", digest[:4], digest[4:6], digest[6:8], digest[8:10], digest[10:16]))
	input["prompt_cache_key"], _ = json.Marshal(hex.EncodeToString(digest[:]))
	raw, err = json.Marshal(input)
	if err != nil {
		return nil, codex.ErrInput
	}
	credential, err := s.accounts.Prepare(ctx, id, s.provider.Refresh)
	if err != nil {
		return nil, err
	}
	execute := func(c accounts.Credential) (*codex.Stream, error) {
		if kind == Chat {
			return s.provider.Chat(ctx, c, raw, outgoing)
		}
		return s.provider.Responses(ctx, c, raw, outgoing, kind == Compact)
	}
	result, err := execute(credential)
	if err != nil {
		return nil, err
	}
	if result.StatusCode == 401 {
		result.Body.Close()
		credential, err = s.accounts.RefreshAfterRejection(ctx, id, credential.AccessToken, s.provider.Refresh)
		if err != nil {
			return nil, err
		}
		result, err = execute(credential)
		if err != nil {
			return nil, err
		}
	}
	if result.StatusCode == 401 {
		_ = s.accounts.RecordUse(ctx, id, credential.AccessToken, false)
	}
	if result.StatusCode >= 200 && result.StatusCode < 300 {
		_ = s.accounts.RecordUse(ctx, id, credential.AccessToken, true)
	}
	return result, nil
}

func (s *Service) Models(ctx context.Context, userID int64) ([]codex.Model, error) {
	id, _, err := s.selectAccount(ctx, userID, "")
	if err != nil {
		return nil, err
	}
	return s.Check(ctx, id)
}

func (s *Service) Check(ctx context.Context, id string) ([]codex.Model, error) {
	credential, err := s.accounts.Prepare(ctx, id, s.provider.Refresh)
	if err != nil {
		return nil, err
	}
	models, err := s.provider.Models(ctx, credential)
	var rejected *codex.UpstreamError
	if errors.As(err, &rejected) && rejected.Status == 401 {
		credential, err = s.accounts.RefreshAfterRejection(ctx, id, credential.AccessToken, s.provider.Refresh)
		if err != nil {
			return nil, err
		}
		models, err = s.provider.Models(ctx, credential)
	}
	if err == nil {
		if err := s.accounts.RecordUse(ctx, id, credential.AccessToken, true); err != nil {
			return nil, err
		}
	}
	if errors.As(err, &rejected) && rejected.Status == 401 {
		_ = s.accounts.RecordUse(ctx, id, credential.AccessToken, false)
	}
	return models, err
}

func (s *Service) selectAccount(ctx context.Context, userID int64, session string) (string, [32]byte, error) {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", userID, session)))
	if userID <= 0 {
		return "", digest, codex.ErrInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	available, err := s.accounts.List(ctx)
	if err != nil {
		return "", digest, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", digest, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	now := time.Now().Unix()
	existing, err := queries.GetAccountAffinity(ctx, db.GetAccountAffinityParams{UserID: userID, SessionHash: digest[:]})
	expires := now + 24*3600
	if err == nil && existing.ExpiresAt > now {
		if err := queries.TouchAccountAffinity(ctx, db.TouchAccountAffinityParams{UserID: userID, SessionHash: digest[:], ExpiresAt: expires, Threshold: expires - 1800}); err != nil {
			return "", digest, err
		}
		if err := tx.Commit(); err != nil {
			return "", digest, err
		}
		// Disabled or deleted accounts fail in Prepare; never silently move existing conversation state.
		return existing.AccountID, digest, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", digest, err
	}
	if err := queries.PruneAccountAffinity(ctx, now); err != nil {
		return "", digest, err
	}
	count, err := queries.CountAccountAffinity(ctx)
	if err != nil {
		return "", digest, err
	}
	if count >= 4096 {
		return "", digest, ErrAffinityLimit
	}
	candidates := make([]string, 0, len(available))
	for _, account := range available {
		if account.Enabled && account.Status != "reauth_required" {
			candidates = append(candidates, account.ID)
		}
	}
	if len(candidates) == 0 {
		return "", digest, ErrNoAccount
	}
	id := candidates[s.next%len(candidates)]
	s.next = (s.next + 1) % len(candidates)
	if err := queries.CreateAccountAffinity(ctx, db.CreateAccountAffinityParams{UserID: userID, SessionHash: digest[:], AccountID: id, ExpiresAt: expires}); err != nil {
		return "", digest, err
	}
	if err := tx.Commit(); err != nil {
		return "", digest, err
	}
	return id, digest, nil
}
