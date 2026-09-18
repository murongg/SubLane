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
