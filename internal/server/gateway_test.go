package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/codex"
	"github.com/murongg/SubLane/internal/gateway"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

type gatewayTransport func(*http.Request) (*http.Response, error)

func (f gatewayTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type forwardFixture struct {
	upgrades      *atomic.Int32
	server        *httptest.Server
	secret        string
	keyID, userID int64
	keys          *apikey.Service
	identity      *auth.Service
}

func newForwardFixture(t *testing.T, handler http.HandlerFunc) forwardFixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	identity, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "owner-test", "owner pass 42"); err != nil {
		t.Fatal(err)
	}
	member, err := identity.CreateMember(ctx, "member-test", "member pass 42")
	if err != nil {
		t.Fatal(err)
	}
	key, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	service := accounts.New(db, key)
	if _, err := service.Authorize(ctx, "Synthetic subscription", accounts.Credential{AccessToken: "synthetic-upstream-access", RefreshToken: "synthetic-upstream-refresh", AccountID: "synthetic-upstream-account", ExpiresAt: time.Now().Add(time.Hour).Unix()}, ""); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	target, _ := url.Parse(upstream.URL)
	client := codex.NewWithTransport(gatewayTransport(func(r *http.Request) (*http.Response, error) {
		copy := r.Clone(r.Context())
		copy.URL.Scheme = target.Scheme
		copy.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(copy)
	}))
	t.Cleanup(client.Close)
	keys := apikey.New(db)
	created, err := keys.Create(ctx, member.ID, "Synthetic client")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(New(Options{Auth: identity, Keys: keys, Accounts: service, Gateway: gateway.New(db, service, client), Ping: db.PingContext}))
	upgrades := &atomic.Int32{}
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateHijacked {
			upgrades.Add(1)
		}
	}
	server.Start()
	t.Cleanup(server.Close)
	return forwardFixture{server: server, upgrades: upgrades, secret: created.Secret, keyID: created.Key.ID, userID: member.ID, keys: keys, identity: identity}
}

func TestGatewayForwardsResponsesAndCancelsUpstream(t *testing.T) {
	canceled := make(chan struct{}, 1)
	fixture := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer synthetic-upstream-access" {
			t.Error("member key leaked upstream")
		}
		if strings.Contains(string(raw), "synthetic-rate") {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			io.WriteString(w, `{"error":"synthetic-private-token"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\"}}\n\n")
		w.(http.Flusher).Flush()
		if strings.Contains(string(raw), "synthetic-cancel") {
			<-r.Context().Done()
			canceled <- struct{}{}
			return
		}
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"output\":[]}}\n\n")
	})
	call := func(ctx context.Context, body string) *http.Response {
		req, _ := http.NewRequestWithContext(ctx, "POST", fixture.server.URL+"/v1/responses", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+fixture.secret)
		req.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	response := call(context.Background(), `{"model":"synthetic-model","input":"synthetic prompt"}`)
	if response.StatusCode != 200 {
		response.Body.Close()
		t.Fatalf("forwarding status: %d", response.StatusCode)
	}
	var completed struct {
		ID string `json:"id"`
	}
	err := json.NewDecoder(response.Body).Decode(&completed)
	response.Body.Close()
	if err != nil || completed.ID != "resp_test" {
		t.Fatal("invalid completed response", err)
	}
	response = call(context.Background(), `{"model":"synthetic-model","input":"synthetic-rate"}`)
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 429 || response.Header.Get("Retry-After") != "2" || strings.Contains(string(data), "synthetic-private-token") {
		t.Fatal("upstream errors were not safely mapped")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	response = call(ctx, `{"model":"synthetic-model","input":"synthetic-cancel","stream":true}`)
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream" {
		response.Body.Close()
		t.Fatal("stream not started")
	}
	first := make([]byte, 64)
	if _, err := response.Body.Read(first); err != nil {
		t.Fatal(err)
	}
	cancel()
	response.Body.Close()
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream request was not canceled")
	}
}
