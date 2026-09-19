package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/upstream"
)

var (
	ErrAffinityUnavailable = errors.New("conversation_account_unavailable")
	ErrNoAccount           = errors.New("no_accounts_available")
	ErrBusy                = errors.New("gateway_busy")
	ErrAffinityLimit       = errors.New("conversation_limit")
)

type Kind uint8

const (
	Responses Kind = iota
	Chat
	Compact
)

type Service struct {
	db           *sql.DB
	queries      *db.Queries
	accounts     *accounts.Service
	provider     *upstream.Client
	slots        chan struct{}
	mu           sync.Mutex
	next         map[string]int
	health       map[string]*Runtime
	memberActive map[int64]int64
	now          func() time.Time
	runContext   context.Context
	stopRuntime  context.CancelFunc
	workers      sync.WaitGroup
	closed       bool
	sequence     int64
	usage        *usageCache
}

func New(ctx context.Context, connection *sql.DB, accounts *accounts.Service, provider *upstream.Client) *Service {
	runContext, stopRuntime := context.WithCancel(ctx)
	return &Service{memberActive: make(map[int64]int64), next: make(map[string]int), health: make(map[string]*Runtime), now: time.Now, runContext: runContext, stopRuntime: stopRuntime, db: connection, queries: db.New(connection), accounts: accounts, provider: provider, slots: make(chan struct{}, 8), usage: newUsageCache(ctx)}
}

func (s *Service) Acquire() (func(), error) {
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, nil
	default:
		return nil, ErrBusy
	}
}

func (s *Service) Open(ctx context.Context, userID, groupID int64, raw []byte, headers http.Header, kind Kind) (exchange *Exchange, failure error) {
	entry, err := s.begin(ctx, userID, groupID, kind)
	if err != nil {
		return nil, err
	}
	ctx = entry.ctx
	defer func() {
		if exchange == nil {
			entry.fail(failure)
		}
	}()
	var input map[string]json.RawMessage
	if json.Unmarshal(raw, &input) != nil || input == nil {
		return nil, upstream.ErrInput
	}
	var model string
	if json.Unmarshal(input["model"], &model) != nil {
		return nil, upstream.ErrInput
	}
	if safeModel.MatchString(model) {
		entry.record.Model = model
	}
	provider := "codex"
	if p, actual, found := strings.Cut(model, "/"); found && accounts.ValidProvider(p) {
		provider, model = p, actual
	}
	if model == "" || len(model) > 128 {
		return nil, upstream.ErrInput
	}
	entry.record.Provider = provider
	input["model"], _ = json.Marshal(model)
	if kind == Compact && provider != "codex" {
		return nil, upstream.ErrInput
	}
	session := headers.Get("Session_id")
	if session == "" && len(input["prompt_cache_key"]) > 0 {
		if json.Unmarshal(input["prompt_cache_key"], &session) != nil {
			return nil, upstream.ErrInput
		}
	}
	if len(session) > 1024 {
		return nil, upstream.ErrInput
	}
	s.mu.Lock()
	if err := s.admitMember(ctx, userID); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	entry.memberLeased = true
	id, digest, err := s.selectAccount(ctx, userID, groupID, session, provider)
	entry.record.AccountID = id
	if err == nil {
		state := s.health[id]
		state.InFlight++
		entry.record.AccountID = id
		entry.revision = state.revision
		entry.leased = true
	}
	s.mu.Unlock()
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
		return nil, upstream.ErrInput
	}
	credential, err := s.accounts.Prepare(ctx, id, s.provider.Refresh)
	if err != nil {
		return nil, err
	}
	execute := func(c accounts.Credential) (*upstream.Stream, error) {
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
	return trackExchange(result, entry), nil
}

func (s *Service) Models(ctx context.Context, userID, groupID int64) ([]upstream.Model, error) {
	allowed, err := poolAccounts(ctx, s.queries, userID, groupID)
	if err != nil {
		return nil, err
	}
	rows, err := s.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	providers := map[string]string{}
	for _, row := range rows {
		if allowed[row.ID] && row.Enabled && row.Status != "reauth_required" && providers[row.Provider] == "" {
			providers[row.Provider] = row.ID
		}
	}
	if len(providers) == 0 {
		return nil, ErrNoAccount
	}
	result := []upstream.Model{}
	var firstError error
	successful := 0
	for _, provider := range []string{"codex", "claude", "antigravity"} {
		if providers[provider] == "" {
			continue
		}
		// Discovery has no conversation state and must not create or reuse an affinity binding.
		models, err := s.Check(ctx, providers[provider])
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if firstError == nil {
				firstError = err
			}
			continue
		}
		successful++
		for _, model := range models {
			if provider == "codex" {
				result = append(result, model)
			}
			model.ID = provider + "/" + model.ID
			result = append(result, model)
		}
	}
	// A temporarily unavailable provider must not hide the healthy providers from clients.
	if successful == 0 && firstError != nil {
		return nil, firstError
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (s *Service) Check(ctx context.Context, id string) ([]upstream.Model, error) {
	return readAccount(ctx, s, id, s.provider.Models)
}

// Account reads share the same refresh owner and stale-token rejection rules as forwarding.
func readAccount[T any](ctx context.Context, s *Service, id string, read func(context.Context, accounts.Credential) (T, error)) (T, error) {
	var empty T
	credential, err := s.accounts.Prepare(ctx, id, s.provider.Refresh)
	if err != nil {
		return empty, err
	}
	result, err := read(ctx, credential)
	var rejected *upstream.UpstreamError
	if errors.As(err, &rejected) && rejected.Status == 401 {
		credential, err = s.accounts.RefreshAfterRejection(ctx, id, credential.AccessToken, s.provider.Refresh)
		if err != nil {
			return empty, err
		}
		result, err = read(ctx, credential)
	}
	if err == nil {
		if err := s.accounts.RecordUse(ctx, id, credential.AccessToken, true); err != nil {
			return empty, err
		}
	}
	if errors.As(err, &rejected) && rejected.Status == 401 {
		_ = s.accounts.RecordUse(ctx, id, credential.AccessToken, false)
	}
	return result, err
}

func (s *Service) selectAccount(ctx context.Context, userID, groupID int64, session, provider string) (string, [32]byte, error) {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", userID, session)))
	// Preserve legacy upstream session/cache IDs for conversations migrated into the default group.
	if groupID != groups.DefaultID {
		digest = sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s", userID, groupID, session)))
	}
	if userID <= 0 || groupID <= 0 {
		return "", digest, upstream.ErrInput
	}
	// Called under s.mu by Open, keeping account selection and reservation atomic.
	if err := s.loadRuntime(ctx); err != nil {
		return "", digest, err
	}
	if session == "" {
		if _, err := rand.Read(digest[:]); err != nil {
			return "", digest, err
		}
	}
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
	allowed, err := poolAccounts(ctx, queries, userID, groupID)
	if err != nil {
		return "", digest, err
	}
	now := s.now().Unix()
	expires := now + 24*3600
	if session != "" {
		existing, err := queries.GetAccountAffinity(ctx, db.GetAccountAffinityParams{GroupID: groupID, UserID: userID, SessionHash: digest[:], Provider: provider})
		if err == nil && existing.ExpiresAt > now {
			// A pool edit is an authorization change, not permission to silently move a conversation.
			if !allowed[existing.AccountID] {
				return existing.AccountID, digest, ErrAffinityUnavailable
			}
			found := false
			for _, account := range available {
				if account.ID == existing.AccountID {
					found = true
					if err := s.accountAdmission(account); err != nil {
						return existing.AccountID, digest, err
					}
					break
				}
			}
			if !found {
				return "", digest, accounts.ErrNotFound
			}
			if err := queries.TouchAccountAffinity(ctx, db.TouchAccountAffinityParams{GroupID: groupID, Provider: provider, UserID: userID, SessionHash: digest[:], ExpiresAt: expires, Threshold: expires - 1800}); err != nil {
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
	}
	candidates := make([]string, 0, len(available))
	busy := false
	var cooling int64
	for _, account := range available {
		if allowed[account.ID] && account.Provider == provider && account.Enabled && account.Status != "reauth_required" {
			if err := s.accountAdmission(account); err != nil {
				if errors.Is(err, ErrAccountBusy) {
					busy = true
				}
				var wait *CoolingError
				if errors.As(err, &wait) && (cooling == 0 || wait.RetryAfter < cooling) {
					cooling = wait.RetryAfter
				}
				continue
			}
			candidates = append(candidates, account.ID)
		}
	}
	if len(candidates) == 0 {
		if busy {
			return "", digest, ErrAccountBusy
		}
		if cooling > 0 {
			return "", digest, &CoolingError{RetryAfter: cooling}
		}
		return "", digest, ErrNoAccount
	}
	cursor := fmt.Sprintf("%d:%s", groupID, provider)
	id := candidates[s.next[cursor]%len(candidates)]
	s.next[cursor] = (s.next[cursor] + 1) % len(candidates)
	if session != "" {
		if err := queries.CreateAccountAffinity(ctx, db.CreateAccountAffinityParams{GroupID: groupID, Provider: provider, UserID: userID, SessionHash: digest[:], AccountID: id, ExpiresAt: expires}); err != nil {
			return "", digest, err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", digest, err
	}
	return id, digest, nil
}

func poolAccounts(ctx context.Context, queries *db.Queries, userID, groupID int64) (map[string]bool, error) {
	allowed, err := queries.CanUseGroup(ctx, db.CanUseGroupParams{UserID: userID, GroupID: groupID})
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, groups.ErrUnavailable
	}
	ids, err := queries.ListGroupAccounts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		result[id] = true
	}
	return result, nil
}
