package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/groups"
)

func TestHiddenGrokModelsForwardWithoutListingOrBypassingPolicy(t *testing.T) {
	var forwarded atomic.Int32
	f := newProviderFixture(t, "xai", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			io.WriteString(w, `{"data":[{"id":"synthetic-model"},{"id":"synthetic-picker-alias","model":"synthetic-hidden","_meta":{"hidden":true}}]}`)
			return
		}
		var input struct{ Model string }
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Model != "synthetic-hidden" || r.Header.Get("X-XAI-Token-Auth") != "xai-grok-cli" {
			t.Error("hidden model lost its native ID or subscription authentication", input.Model)
		}
		forwarded.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic-response\",\"object\":\"response\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")
	})
	ctx := context.Background()
	rows, err := f.accounts.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	id := rows[0].ID
	saved, err := f.accounts.Catalog(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.accounts.SaveCatalog(ctx, id, saved.Revision, saved.Models, time.Now().Unix(), "xai:v1"); err != nil {
		t.Fatal(err)
	}
	if catalog, err := f.forwarding.AccountCatalog(ctx, id, true); err != nil || !catalog.Known || len(catalog.Models) != 1 || catalog.RefreshFailed {
		t.Fatal(catalog, err)
	}
	h := f.server.Config.Handler
	login := func(name, password string) *http.Cookie {
		response := request(h, "POST", "/api/auth/login", "http://example.test", map[string]string{"username": name, "password": password}, nil)
		if response.Code != 200 {
			t.Fatal("login failed", response.Code)
		}
		return response.Result().Cookies()[0]
	}
	admin := login("owner-test", "owner pass 42")
	member := login("member-test", "member pass 42")
	assertListings := func() {
		t.Helper()
		response, raw := messageRequest(t, f, "GET", "/v1/models", "", http.Header{"Authorization": {"Bearer " + f.secret}})
		if response.StatusCode != 200 || strings.Contains(raw, "synthetic-hidden") || strings.Contains(raw, "synthetic-picker-alias") {
			t.Fatal("hidden model entered public list", response.StatusCode, raw)
		}
		for _, test := range []struct {
			path   string
			cookie *http.Cookie
		}{{"/api/accounts/" + id + "/models", admin}, {"/api/groups/1/models", admin}, {fmt.Sprintf("/api/keys/%d/models", f.keyID), member}} {
			response := request(h, "GET", test.path, "", nil, test.cookie)
			if response.Code != 200 || strings.Contains(response.Body.String(), "synthetic-hidden") || strings.Contains(response.Body.String(), "synthetic-picker-alias") {
				t.Fatal("hidden capability leaked into management list", response.Code, response.Body.String())
			}
		}
	}
	assertListings()
	call := func(model string) int {
		response, _ := messageRequest(t, f, "POST", "/v1/responses", fmt.Sprintf(`{"model":%q,"input":"Synthetic prompt"}`, model), http.Header{"Authorization": {"Bearer " + f.secret}})
		return response.StatusCode
	}
	if status := call("synthetic-hidden"); status != 200 || forwarded.Load() != 1 {
		t.Fatal("hidden model did not forward", status, forwarded.Load())
	}
	conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(f.server.URL, "http://", "ws://", 1)+"/v1/responses", http.Header{"Authorization": {"Bearer " + f.secret}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	turn := func(want string) {
		t.Helper()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteJSON(map[string]any{"type": "response.create", "model": "synthetic-hidden", "input": "Synthetic prompt"}); err != nil {
			t.Fatal(err)
		}
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), want) {
				return
			}
			if strings.Contains(string(raw), `"type":"error"`) {
				t.Fatal("unexpected WebSocket rejection", string(raw))
			}
		}
	}
	turn("response.completed")
	for _, model := range []string{"synthetic-unknown", "synthetic-picker-alias"} {
		if status := call(model); status != 404 || forwarded.Load() != 2 {
			t.Fatal("unadvertised routing ID forwarded", status, forwarded.Load())
		}
	}
	input := groups.Input{Name: "Synthetic Grok", Enabled: true, AccountIDs: []string{id}, ModelPolicy: &groups.ModelPolicy{Restricted: true, Models: []string{"synthetic-model"}}}
	if _, err := f.groups.Save(ctx, 1, input); err != nil {
		t.Fatal(err)
	}
	if status := call("synthetic-hidden"); status != 403 || forwarded.Load() != 2 {
		t.Fatal("hidden model bypassed model policy", status, forwarded.Load())
	}
	turn("model_not_allowed")
	input.ModelPolicy.Models = []string{"synthetic-hidden"}
	if _, err := f.groups.Save(ctx, 1, input); err != nil {
		t.Fatal(err)
	}
	assertListings()
	if status := call("synthetic-hidden"); status != 200 || forwarded.Load() != 3 {
		t.Fatal("explicit permission could not call a hidden model", status, forwarded.Load())
	}
	if err := f.groups.SetMemberGroups(ctx, f.userID, []int64{}); err != nil {
		t.Fatal(err)
	}
	if status := call("synthetic-hidden"); status != 401 || forwarded.Load() != 3 {
		t.Fatal("hidden model bypassed group access", status, forwarded.Load())
	}
	if err := f.groups.SetMemberGroups(ctx, f.userID, []int64{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.keys.Update(ctx, f.userID, f.keyID, apikey.UpdateInput{Name: "Synthetic key", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if status := call("synthetic-hidden"); status != 401 || forwarded.Load() != 3 {
		t.Fatal("hidden model bypassed paused key", status, forwarded.Load())
	}
	turn("invalid_api_key")
	if forwarded.Load() != 3 {
		t.Fatal("WebSocket turn bypassed a changed policy or paused key", forwarded.Load())
	}
}
