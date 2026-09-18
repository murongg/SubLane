package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
)

func TestOAuthExchangeAndRefreshUsePinnedEndpoints(t *testing.T) {
	idToken := "e30." + base64.RawURLEncoding.EncodeToString([]byte(`{"email":"member@example.test","https://api.openai.com/auth":{"chatgpt_account_id":"upstream-test","chatgpt_plan_type":"plus"}}`)) + ".synthetic"
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/token" {
			t.Error("wrong token endpoint")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("client_id") != clientID {
			t.Error("wrong public OAuth client")
		}
		if calls == 1 && (r.Form.Get("code_verifier") != "synthetic-verifier" || r.Form.Get("redirect_uri") != redirectURI) {
			t.Error("PKCE missing")
		}
		if calls == 2 && r.Form.Get("refresh_token") != "synthetic-refresh" {
			t.Error("wrong refresh token")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "synthetic-access", "refresh_token": "synthetic-refresh", "id_token": idToken, "expires_in": 3600})
	}))
	defer upstream.Close()
	client := New()
	client.tokenURL = upstream.URL + "/token"
	started, _ := url.Parse(client.AuthorizationURL("synthetic-state", "synthetic-challenge"))
	if started.Host != "auth.openai.com" || started.Query().Get("code_challenge_method") != "S256" || started.Query().Get("state") != "synthetic-state" {
		t.Fatal("invalid authorization URL")
	}
	credential, err := client.Exchange(context.Background(), "synthetic-code", "synthetic-verifier")
	if err != nil || credential.AccountID != "upstream-test" || credential.Email != "member@example.test" || credential.ExpiresAt < time.Now().Unix() {
		t.Fatal("token exchange failed", err)
	}
	if _, err := client.Refresh(context.Background(), credential); err != nil {
		t.Fatal(err)
	}
}

func TestSDKTranslationAndUpstreamHeaderIsolation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-upstream" || r.Header.Get("Chatgpt-Account-Id") != "upstream-test" {
			t.Error("upstream identity missing")
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("X-Untrusted") != "" {
			t.Error("client headers leaked")
		}
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"models":[{"slug":"synthetic-model","visibility":"list"}]}`)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["stream"] != true || body["store"] != false || body["model"] != "synthetic-model" {
			t.Error("SDK translation missing")
		}
		if _, ok := body["max_output_tokens"]; ok {
			t.Error("unsupported field forwarded")
		}
		if _, ok := body["previous_response_id"]; ok {
			t.Error("unsupported continuation forwarded")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_synthetic\",\"output\":[]}}\n\n")
	}))
	defer upstream.Close()
	client := New()
	client.baseURL = upstream.URL
	credential := accounts.Credential{AccessToken: "synthetic-upstream", AccountID: "upstream-test"}
	headers := http.Header{"Authorization": {"Bearer synthetic-member-key"}, "Cookie": {"synthetic-cookie"}, "X-Untrusted": {"synthetic"}}
	response, err := client.Responses(context.Background(), credential, []byte(`{"model":"synthetic-model","input":"synthetic prompt","max_output_tokens":50}`), headers, false)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("forwarding failed")
	}
	models, err := client.Models(context.Background(), credential)
	if err != nil || len(models) != 1 || models[0].ID != "synthetic-model" {
		t.Fatal("models not discovered", err)
	}
	if _, err := client.Responses(context.Background(), credential, []byte(`{"model":"synthetic-model","previous_response_id":"unknown-response","input":[]}`), nil, false); !errors.Is(err, ErrContinuation) {
		t.Fatal("stateless continuation silently dropped", err)
	}
}

func TestTokenErrorsDoNotExposeProviderBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, `{"error":"invalid_grant","detail":"synthetic-secret"}`)
	}))
	defer upstream.Close()
	client := New()
	client.tokenURL = upstream.URL
	_, err := client.Refresh(context.Background(), accounts.Credential{RefreshToken: "synthetic-refresh"})
	if !errors.Is(err, accounts.ErrReauthorize) || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatal("unsafe provider error", err)
	}
}
