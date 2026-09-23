package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

type fakeProvider struct {
	challenge, verifier string
	exchanges           int
}

func (p *fakeProvider) AuthorizationURL(state, challenge string) string {
	p.challenge = challenge
	return "https://auth.example.test/authorize?state=" + url.QueryEscape(state) + "&code_challenge=" + url.QueryEscape(challenge)
}
func (p *fakeProvider) Exchange(ctx context.Context, code, verifier string) (accounts.Credential, error) {
	p.exchanges++
	p.verifier = verifier
	return accounts.Credential{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", AccountID: "upstream-test", ExpiresAt: time.Now().Add(time.Hour).Unix()}, nil
}

func TestOAuthStateIsSessionBoundSingleUseAndPKCEProtected(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "oauth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	identity, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "synthetic-admin", "synthetic-password", "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	key, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{}
	flow := New(accounts.New(db, key), provider)
	started, err := flow.Begin(ctx, "session-one", "Test OAuth", "")
	if err != nil {
		t.Fatal(err)
	}
	callback := "http://localhost:1455/auth/callback?state=" + started.State + "&code=synthetic-code"
	if _, err := flow.Finish(ctx, "session-two", started.State, callback); !errors.Is(err, ErrState) {
		t.Fatal("session ownership ignored", err)
	}
	if provider.exchanges != 0 {
		t.Fatal("foreign session exchanged a code")
	}
	if _, err := flow.Finish(ctx, "session-one", started.State, "https://other.example.test/?state="+started.State+"&code=x"); !errors.Is(err, ErrCallback) {
		t.Fatal("foreign callback accepted", err)
	}
	account, err := flow.Finish(ctx, "session-one", started.State, callback)
	if err != nil || account.Status != "ready" {
		t.Fatal("authorization failed", err)
	}
	digest := sha256.Sum256([]byte(provider.verifier))
	if base64.RawURLEncoding.EncodeToString(digest[:]) != provider.challenge {
		t.Fatal("PKCE mismatch")
	}
	if _, err := flow.Finish(ctx, "session-one", started.State, callback); !errors.Is(err, ErrState) {
		t.Fatal("callback replay accepted", err)
	}
	if provider.exchanges != 1 {
		t.Fatal("code exchanged more than once")
	}
	started, err = flow.Begin(ctx, "session-one", "Expired", "")
	if err != nil {
		t.Fatal(err)
	}
	flow.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	if _, err := flow.Finish(ctx, "session-one", started.State, "http://localhost:1455/auth/callback?state="+started.State+"&code=x"); !errors.Is(err, ErrState) {
		t.Fatal("expired state accepted", err)
	}
}

type subscriptionProvider struct {
	fakeProvider
	kind string
}

func (p *subscriptionProvider) AuthorizationURLFor(provider, state, challenge string) (string, error) {
	p.kind = provider
	return p.AuthorizationURL(state, challenge), nil
}
func (p *subscriptionProvider) ExchangeFor(ctx context.Context, provider, code, state, verifier string) (accounts.Credential, error) {
	c, err := p.Exchange(ctx, code, verifier)
	c.Provider = provider
	return c, err
}

func TestProviderAuthorizationBindsCallbackSessionAndReauthorization(t *testing.T) {
	for _, kind := range []string{"claude", "antigravity"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			identity, err := auth.New(db)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := identity.Setup(ctx, "synthetic-admin", "synthetic-password", "Synthetic workspace"); err != nil {
				t.Fatal(err)
			}
			cipher, err := vault.Open(filepath.Join(dir, "key"), true)
			if err != nil {
				t.Fatal(err)
			}
			provider := &subscriptionProvider{}
			flow := New(accounts.New(db, cipher), provider)
			started, err := flow.BeginProvider(ctx, kind, "synthetic-owner", "Synthetic", "")
			if err != nil {
				t.Fatal(err)
			}
			callback := started.CallbackURL + "?state=" + started.State + "&code=synthetic-code"
			if _, err := flow.Finish(ctx, "another-owner", started.State, callback); !errors.Is(err, ErrState) {
				t.Fatal("foreign session accepted", err)
			}
			if _, err := flow.Finish(ctx, "synthetic-owner", started.State, "http://localhost:1455/auth/callback?state="+started.State+"&code=x"); !errors.Is(err, ErrCallback) {
				t.Fatal("wrong provider callback accepted", err)
			}
			row, err := flow.Finish(ctx, "synthetic-owner", started.State, callback)
			if err != nil || row.Provider != kind {
				t.Fatal("provider lost", err)
			}
			if _, err := flow.Finish(ctx, "synthetic-owner", started.State, callback); !errors.Is(err, ErrState) {
				t.Fatal("replay accepted", err)
			}
			if _, err := flow.BeginProvider(ctx, "codex", "synthetic-owner", "Replacement", row.ID); !errors.Is(err, accounts.ErrIdentity) {
				t.Fatal("reauthorization crossed provider", err)
			}
			if provider.exchanges != 1 {
				t.Fatal("unexpected exchanges", provider.exchanges)
			}
		})
	}
}
