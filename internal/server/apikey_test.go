package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/upstream"
)

func TestAPIKeyManagementUsesAdministratorBoundary(t *testing.T) {
	fixture := newForwardFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected upstream inference") })
	h := New(Options{Auth: fixture.identity, Accounts: fixture.accounts})
	origin := "http://example.test"
	login := request(h, "POST", "/api/auth/login", origin, map[string]string{"username": "owner-test", "password": "owner pass 42"}, nil)
	owner := login.Result().Cookies()[0]
	input := map[string]string{"name": "Synthetic API", "api_key": "synthetic-api-key", "base_url": "https://relay.example.test/v1"}
	if result := request(h, "POST", "/api/accounts/api-key", origin, input, nil); result.Code != 401 {
		t.Fatal("anonymous API upstream write", result.Code)
	}
	member, err := fixture.identity.Login(context.Background(), "member-test", "member pass 42")
	if err != nil {
		t.Fatal(err)
	}
	memberCookie := &http.Cookie{Name: "sublane_session", Value: member.Token}
	if result := request(h, "POST", "/api/accounts/api-key", origin, input, memberCookie); result.Code != 403 {
		t.Fatal("member API upstream write", result.Code)
	}
	if result := request(h, "POST", "/api/accounts/api-key", "https://other.example.test", input, owner); result.Code != 403 {
		t.Fatal("cross-origin API upstream write", result.Code)
	}
	created := request(h, "POST", "/api/accounts/api-key", origin, input, owner)
	if created.Code != 201 {
		t.Fatal("API upstream creation", created.Code, created.Body.String())
	}
	var account accounts.Account
	if json.Unmarshal(created.Body.Bytes(), &account) != nil || account.Provider != "openai" || account.BaseURL != input["base_url"] || strings.Contains(created.Body.String(), "synthetic-api-key") {
		t.Fatal("API upstream metadata", created.Body.String())
	}
	input["replace_id"] = account.ID
	input["api_key"] = "synthetic-rotated-key"
	updated := request(h, "POST", "/api/accounts/api-key", origin, input, owner)
	if updated.Code != 201 {
		t.Fatal("API upstream rotation", updated.Code, updated.Body.String())
	}
	listed := request(h, "GET", "/api/accounts", "", nil, owner)
	if listed.Code != 200 || strings.Contains(listed.Body.String(), "synthetic-rotated-key") {
		t.Fatal("API secret disclosed", listed.Body.String())
	}
}

func TestAPIKeyGatewaySharesPoolPolicyAndRejectionLifecycle(t *testing.T) {
	ctx := context.Background()
	var key atomic.Value
	key.Store("synthetic-api-key")
	var reject atomic.Bool
	fixture := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key.Load().(string) {
			t.Error("wrong upstream key")
		}
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"openai/synthetic-model"}]}`)
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Error("unexpected provider endpoint", r.URL.Path)
		}
		if reject.Load() {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"id\":\"synthetic-chat\",\"object\":\"chat.completion.chunk\",\"model\":\"openai/synthetic-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Synthetic output\"},\"finish_reason\":null}]}\n\n")
		io.WriteString(w, "data: {\"id\":\"synthetic-chat\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\ndata: [DONE]\n\n")
	})
	raw := `{"api_key":"synthetic-api-key","base_url":"https://relay.example.test/v1"}`
	account, err := fixture.accounts.ImportProvider(ctx, "openai", "Synthetic API", []byte(raw), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.forwarding.Check(ctx, account.ID); err != nil {
		t.Fatal("API model discovery", err)
	}
	if _, err := fixture.forwarding.Usage(ctx, account.ID); !errors.Is(err, upstream.ErrUsageUnsupported) {
		t.Fatal("API account exposed subscription quota", err)
	}
	pool, err := fixture.groups.Save(ctx, 1, groups.Input{Name: "Synthetic pool", Enabled: true, AccountIDs: []string{account.ID}})
	if err != nil || pool.AccountCount != 1 {
		t.Fatal("API pool eligibility", pool, err)
	}
	call := func(path, body string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest("POST", fixture.server.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+fixture.secret)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Session_id", "synthetic-api-conversation")
		response, err := fixture.server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, string(data)
	}
	for _, stream := range []bool{false, true} {
		for _, protocol := range []string{"responses", "chat/completions"} {
			body := map[string]any{"model": "openai/synthetic-model", "stream": stream, "input": "Synthetic input"}
			if protocol == "chat/completions" {
				delete(body, "input")
				body["messages"] = []map[string]string{{"role": "user", "content": "Synthetic input"}}
			}
			encoded, _ := json.Marshal(body)
			if status, result := call("/v1/"+protocol, string(encoded)); status != 200 || !strings.Contains(result, "Synthetic output") {
				t.Fatal("API gateway response", protocol, stream, status, result)
			}
		}
	}
	conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(fixture.server.URL, "http://", "ws://", 1)+"/v1/responses", http.Header{"Authorization": {"Bearer " + fixture.secret}})
	if err != nil {
		t.Fatal("API WebSocket handshake", err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"openai/synthetic-model","input":"Synthetic input"}`)); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	var websocketOutput strings.Builder
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			conn.Close()
			t.Fatal("API WebSocket response", err)
		}
		websocketOutput.Write(raw)
		var event struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &event) != nil || event.Type == "error" {
			conn.Close()
			t.Fatal("API WebSocket error", string(raw))
		}
		if event.Type == "response.completed" {
			break
		}
	}
	conn.Close()
	if !strings.Contains(websocketOutput.String(), "Synthetic output") {
		t.Fatal("API WebSocket output missing")
	}
	if _, err := fixture.groups.Save(ctx, 1, groups.Input{Name: pool.Name, Enabled: true, AccountIDs: []string{account.ID}, ModelPolicy: &groups.ModelPolicy{Restricted: true, Models: []string{"other-model"}}}); err != nil {
		t.Fatal(err)
	}
	if status, _ := call("/v1/responses", `{"model":"openai/synthetic-model","input":"Synthetic input"}`); status != 403 {
		t.Fatal("API model policy bypassed", status)
	}
	if _, err := fixture.groups.Save(ctx, 1, groups.Input{Name: pool.Name, Enabled: true, AccountIDs: []string{account.ID}, ModelPolicy: &groups.ModelPolicy{Models: []string{}}}); err != nil {
		t.Fatal(err)
	}
	reject.Store(true)
	if status, _ := call("/v1/responses", `{"model":"openai/synthetic-model","input":"Synthetic input"}`); status == 200 {
		t.Fatal("rejected API key succeeded")
	}
	current, err := fixture.accounts.Get(ctx, account.ID)
	if err != nil || current.Status != "reauth_required" {
		t.Fatal("API rejection not persisted", current, err)
	}
	key.Store("synthetic-rotated-key")
	reject.Store(false)
	raw = `{"api_key":"synthetic-rotated-key","base_url":"https://relay.example.test/v1"}`
	if _, err := fixture.accounts.ImportProvider(ctx, "openai", account.Name, []byte(raw), account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.forwarding.Check(ctx, account.ID); err != nil {
		t.Fatal(err)
	}
	if status, result := call("/v1/responses", `{"model":"openai/synthetic-model","input":"Synthetic input"}`); status != 200 {
		t.Fatal("rotated API conversation lost", status, result)
	}
}
