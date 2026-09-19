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
