package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
)

func TestAntigravitySDKPreservesOAuthBoundaries(t *testing.T) {
	for _, mode := range []string{"exchange", "refresh"} {
		for _, test := range []struct {
			name   string
			status int
			body   string
			want   error
		}{
			{"rejected", 400, `{"error":"invalid_grant","detail":"synthetic-private-body"}`, accounts.ErrReauthorize},
			{"oversized", 200, strings.Repeat("x", 129<<10), ErrResponse},
			{"invalid-expiry", 200, `{"access_token":"synthetic-access","expires_in":0}`, ErrResponse},
			{"redirect", 302, `{}`, ErrUpstream},
		} {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				calls := 0
				client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.URL.Host != "oauth2.googleapis.com" {
						t.Error("OAuth endpoint changed")
					}
					return &http.Response{StatusCode: test.status, Header: http.Header{"Location": {"https://redirect.example.test/"}}, Body: io.NopCloser(strings.NewReader(test.body))}, nil
				}))
				defer client.Close()
				var err error
				if mode == "exchange" {
					_, err = client.ExchangeFor(context.Background(), "antigravity", "synthetic-code", "synthetic-state", "")
				} else {
					_, err = client.Refresh(context.Background(), antigravityFixture())
				}
				if !errors.Is(err, test.want) || calls != 1 {
					t.Fatal("boundary not preserved", calls, err)
				}
			})
		}
	}
}

func TestAntigravitySDKRefreshRespectsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	defer client.Close()
	result := make(chan error, 1)
	go func() { _, err := client.Refresh(ctx, antigravityFixture()); result <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("refresh did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation lost", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SDK detached from caller cancellation")
	}
}

func antigravityFixture() accounts.Credential {
	return accounts.Credential{Provider: "antigravity", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", Email: "member@example.test", AccountID: "member@example.test", ExpiresAt: time.Now().Add(time.Hour).Unix(), Metadata: map[string]json.RawMessage{"project_id": json.RawMessage(`"synthetic-project"`)}}
}

func TestAntigravityDiscoverySeparatesInternalCapabilities(t *testing.T) {
	for _, test := range []struct {
		name, body string
		visible    int
	}{
		{"mixed", `{"models":{"synthetic-visible":{},"synthetic-internal":{"isInternal":true},"synthetic-public":{"isInternal":false}}}`, 2},
		{"internal only", `{"models":{"synthetic-internal":{"isInternal":true}}}`, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1internal:fetchAvailableModels" || r.Header.Get("Authorization") != "Bearer synthetic-access" {
					t.Error("discovery lost its provider or credential boundary")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			}))
			defer client.Close()
			discovery, err := client.Discover(context.Background(), antigravityFixture())
			if err != nil || len(discovery.Models) != test.visible+1 || discovery.Source != "antigravity:v2" {
				t.Fatalf("capabilities or refresh source lost: %+v, %v", discovery, err)
			}
			for _, model := range discovery.Models {
				if model.Hidden != (model.ID == "synthetic-internal") {
					t.Fatalf("wrong visibility: %+v", model)
				}
			}
			models, err := client.Models(context.Background(), antigravityFixture())
			if err != nil || len(models) != test.visible {
				t.Fatalf("internal model entered display list: %+v, %v", models, err)
			}
		})
	}
}

func TestAntigravityDiscoveryRejectsMalformedVisibility(t *testing.T) {
	for _, metadata := range []string{`{"isInternal":"true"}`, `[]`, `42`} {
		client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"models":{"synthetic-model":` + metadata + `}}`))}, nil
		}))
		_, err := client.Discover(context.Background(), antigravityFixture())
		client.Close()
		if !errors.Is(err, ErrResponse) {
			t.Fatalf("malformed model metadata accepted: %s, %v", metadata, err)
		}
	}
}
