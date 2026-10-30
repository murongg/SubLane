package main

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/tenants"
	"github.com/murongg/SubLane/internal/upstream"
	"github.com/murongg/SubLane/internal/vault"
)

func TestRegistrySchedulesOptedInAccountWithoutBrowserActivity(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	conn, err := storage.Open(ctx, filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	identity, err := auth.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "synthetic-owner", "synthetic-password", "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	account, err := accounts.New(conn, cipher).Authorize(ctx, "Synthetic account", accounts.Credential{AccountID: "synthetic-subject", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE accounts SET reset_config='{"exhausted":true,"daily_limit":1,"frequency":"daily","time":"09:00","weekday":1}' WHERE id=?`, account.ID); err != nil {
		t.Fatal(err)
	}
	consumed := make(chan struct{}, 1)
	var redeemed atomic.Bool
	client := upstream.NewWithTransport(subscriptionTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"available_count":2}`
		if r.URL.Path == "/backend-api/wham/rate-limit-reset-credits" {
			body = `{"available_count":2,"credits":[{"id":"synthetic-card","status":"available","reset_type":"codex_rate_limits","granted_at":"2026-01-01T00:00:00Z","expires_at":null}]}`
		}
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			if redeemed.Load() {
				body = `{"rate_limit":{"allowed":true}}`
			} else {
				body = `{"rate_limit":{"allowed":false}}`
			}
		case "/backend-api/wham/rate-limit-reset-credits/consume":
			redeemed.Store(true)
			consumed <- struct{}{}
			body = `{"code":"reset","windows_reset":1}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	defer client.Close()
	registry := &tenantRegistry{ctx: ctx, db: conn, vault: cipher, auth: identity, provider: client, tenants: tenants.New(conn)}
	registry.Start()
	defer registry.Close()
	if registry.resetCredits == nil {
		t.Fatal("scheduler missing")
	}
	select {
	case <-consumed:
	case <-time.After(5 * time.Second):
		t.Fatal("background account never redeemed")
	}
}
