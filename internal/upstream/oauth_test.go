package upstream

import (
	"context"
	"encoding/json"
	"github.com/murongg/SubLane/internal/accounts"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestProviderOAuthUsesBoundedPinnedEndpoints(t *testing.T) {
	for _, provider := range []string{"claude", "antigravity"} {
		t.Run(provider, func(t *testing.T) {
			client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
				var body string
				switch r.URL.Host {
				case "platform.claude.com", "oauth2.googleapis.com":
					body = `{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","expires_in":3600,"account":{"uuid":"synthetic-account","email_address":"member@example.test"},"organization":{"uuid":"synthetic-organization"}}`
				case "www.googleapis.com":
					body = `{"email":"member@example.test"}`
				default:
					t.Errorf("unexpected OAuth endpoint %s", r.URL.Host)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			defer client.Close()
			raw, err := client.AuthorizationURLFor(provider, "synthetic-state", "synthetic-challenge")
			if err != nil {
				t.Fatal(err)
			}
			u, _ := url.Parse(raw)
			if u.Query().Get("state") != "synthetic-state" || u.Query().Get("redirect_uri") == "" {
				t.Fatal("OAuth state or redirect missing")
			}
			credential, err := client.ExchangeFor(context.Background(), provider, "synthetic-code", "synthetic-state", "synthetic-verifier")
			if err != nil || credential.Kind() != provider || credential.Email != "member@example.test" {
				t.Fatal("provider exchange failed", err)
			}
		})
	}
}

func TestAntigravityRefreshRotatesRejectedUnexpiredToken(t *testing.T) {
	calls := 0
	client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "cloudcode-pa.googleapis.com" && r.URL.Path == "/v1internal:loadCodeAssist" {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		calls++
		if r.URL.Host != "oauth2.googleapis.com" {
			t.Errorf("unexpected refresh host %s", r.URL.Host)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"synthetic-new","refresh_token":"synthetic-next","expires_in":3600}`))}, nil
	}))
	defer client.Close()
	old := accounts.Credential{Provider: "antigravity", AccessToken: "synthetic-old", RefreshToken: "synthetic-refresh", AccountID: "member@example.test", Email: "member@example.test", ExpiresAt: time.Now().Add(time.Hour).Unix(), Metadata: map[string]json.RawMessage{"project_id": json.RawMessage(`"synthetic-project"`)}}
	fresh, err := client.Refresh(context.Background(), old)
	if err != nil || calls != 1 || fresh.AccessToken != "synthetic-new" {
		t.Fatal("rejected access token was not rotated", calls, err)
	}
}

func TestAntigravityExchangeUsesVerifiedUserInfo(t *testing.T) {
	profileCalls := 0
	client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","expires_in":3600,"account":{"email_address":"wrong@example.test"}}`
		if r.URL.Host == "www.googleapis.com" {
			profileCalls++
			body = `{"email":"verified@example.test"}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	defer client.Close()
	credential, err := client.ExchangeFor(context.Background(), "antigravity", "synthetic-code", "synthetic-state", "synthetic-verifier")
	if err != nil || credential.Email != "verified@example.test" || profileCalls != 1 {
		t.Fatal("identity did not come from userinfo", err)
	}
}
