package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
)

var (
	ErrState    = errors.New("oauth_state_invalid")
	ErrCallback = errors.New("oauth_callback_invalid")
	ErrDenied   = errors.New("oauth_access_denied")
	ErrBusy     = errors.New("oauth_busy")
)

type Provider interface {
	AuthorizationURL(state, challenge string) string
	Exchange(ctx context.Context, code, verifier string) (accounts.Credential, error)
}

type Authorization struct {
	URL       string `json:"url"`
	State     string `json:"state"`
	ExpiresAt int64  `json:"expires_at"`
}

type pending struct {
	owner                     [32]byte
	verifier, name, replaceID string
	expires                   time.Time
}

type Flow struct {
	accounts *accounts.Service
	provider Provider
	mu       sync.Mutex
	pending  map[string]pending
	now      func() time.Time
}

func New(service *accounts.Service, provider Provider) *Flow {
	return &Flow{accounts: service, provider: provider, pending: make(map[string]pending), now: time.Now}
}

func (f *Flow) Begin(ctx context.Context, session, name, replaceID string) (Authorization, error) {
	if session == "" {
		return Authorization{}, ErrState
	}
	name, err := accounts.NormalizeName(name)
	if err != nil {
		return Authorization{}, err
	}
	if replaceID != "" {
		if _, err := f.accounts.Get(ctx, replaceID); err != nil {
			return Authorization{}, err
		}
	}
	owner := sha256.Sum256([]byte(session))
	state, verifier := randomToken(), randomToken()
	expires := f.now().Add(10 * time.Minute)
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, value := range f.pending {
		if !value.expires.After(f.now()) || value.owner == owner {
			delete(f.pending, key)
		}
	}
	if len(f.pending) >= 8 {
		return Authorization{}, ErrBusy
	}
	f.pending[state] = pending{owner: owner, verifier: verifier, name: name, replaceID: replaceID, expires: expires}
	challenge := sha256.Sum256([]byte(verifier))
	return Authorization{URL: f.provider.AuthorizationURL(state, base64.RawURLEncoding.EncodeToString(challenge[:])), State: state, ExpiresAt: expires.Unix()}, nil
}

func (f *Flow) Finish(ctx context.Context, session, state, callback string) (accounts.Account, error) {
	owner := sha256.Sum256([]byte(session))
	f.mu.Lock()
	entry, ok := f.pending[state]
	if !ok || entry.owner != owner || !entry.expires.After(f.now()) {
		if ok && !entry.expires.After(f.now()) {
			delete(f.pending, state)
		}
		f.mu.Unlock()
		return accounts.Account{}, ErrState
	}
	code, err := callbackCode(callback, state)
	if errors.Is(err, ErrCallback) {
		f.mu.Unlock()
		return accounts.Account{}, err
	}
	// Consume before network IO: concurrent submissions and retries must never exchange the same code twice.
	delete(f.pending, state)
	f.mu.Unlock()
	if err != nil {
		return accounts.Account{}, err
	}
	credential, err := f.provider.Exchange(ctx, code, entry.verifier)
	if err != nil {
		return accounts.Account{}, err
	}
	return f.accounts.Authorize(ctx, entry.name, credential, entry.replaceID)
}

func (f *Flow) Cancel(session, state string) error {
	owner := sha256.Sum256([]byte(session))
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.pending[state]
	if !ok {
		return nil
	}
	if entry.owner != owner {
		return ErrState
	}
	delete(f.pending, state)
	return nil
}

func callbackCode(callback, state string) (string, error) {
	if len(callback) > 8192 {
		return "", ErrCallback
	}
	parsed, err := url.Parse(callback)
	if err != nil || parsed.Scheme != "http" || parsed.Host != "localhost:1455" || parsed.Path != "/auth/callback" || parsed.User != nil || parsed.Fragment != "" {
		return "", ErrCallback
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query["state"]) != 1 || query.Get("state") != state {
		return "", ErrCallback
	}
	if query.Get("error") != "" {
		return "", ErrDenied
	}
	if len(query["code"]) != 1 || query.Get("code") == "" {
		return "", ErrCallback
	}
	return query.Get("code"), nil
}

func randomToken() string {
	value := make([]byte, 32)
	_, _ = rand.Read(value)
	return base64.RawURLEncoding.EncodeToString(value)
}
