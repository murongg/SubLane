package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/upstream"
)

func TestReviewParentAffinityAcrossHTTPAndWebsocket(t *testing.T) {
	type observation struct{ account, cache string }
	observed := make(chan observation, 8)
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Cache string `json:"prompt_cache_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		observed <- observation{r.Header.Get("Chatgpt-Account-Id"), input.Cache}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_synthetic\",\"output\":[]}}\n\n")
	})
	ctx := context.Background()
	rows, err := f.accounts.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	second, err := f.accounts.Authorize(ctx, "Synthetic second", accounts.Credential{AccountID: "synthetic-second", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{rows[0].ID, second.ID}
	for _, id := range ids {
		if err := f.accounts.SaveCatalog(ctx, id, 0, []string{"synthetic-model", "codex-auto-review"}, time.Now().Unix(), upstream.CatalogSource("codex"), "codex-auto-review"); err != nil {
			t.Fatal(err)
		}
	}
	pool := groups.Input{Name: "Default", Enabled: true, AccountIDs: ids}
	if _, err := f.groups.Save(ctx, 1, pool); err != nil {
		t.Fatal(err)
	}
	readObservation := func() observation {
		t.Helper()
		select {
		case value := <-observed:
			return value
		case <-time.After(3 * time.Second):
			t.Fatal("request did not reach the synthetic upstream")
			return observation{}
		}
	}
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+f.secret)
	h.Set("Session_id", "synthetic-parent")
	if response, body := messageRequest(t, f, "POST", "/v1/responses", `{"model":"synthetic-model","input":"Synthetic question"}`, h); response.StatusCode != 200 {
		t.Fatal(response.StatusCode, body)
	}
	parent := readObservation()
	h.Set("Session_id", "synthetic-review-ws")
	conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(f.server.URL, "http://", "ws://", 1)+"/v1/responses", h)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	frame := map[string]any{
		"type": "response.create", "model": "codex-auto-review", "input": "Synthetic operation",
		"client_metadata": map[string]string{"x-codex-turn-metadata": `{"parent_thread_id":"synthetic-parent","subagent_kind":"guardian"}`},
	}
	readEvent := func(want string) map[string]any {
		t.Helper()
		for {
			var event map[string]any
			if err := conn.ReadJSON(&event); err != nil {
				t.Fatal(err)
			}
			if event["type"] == want {
				return event
			}
			if event["type"] == "error" {
				t.Fatal("unexpected WebSocket rejection", event)
			}
		}
	}
	if err := conn.WriteJSON(frame); err != nil {
		t.Fatal(err)
	}
	readEvent("response.completed")
	child := readObservation()
	if child.account != parent.account || parent.cache == "" || child.cache == parent.cache || child.cache == "synthetic-review-ws" {
		t.Fatal("review lost account affinity or reused its parent's upstream session", parent, child)
	}
	h.Set("Session_id", "synthetic-review-http")
	h.Set("X-Codex-Parent-Thread-Id", "synthetic-parent")
	h.Set("X-OpenAI-Subagent", "review")
	raw := `{"model":"codex-auto-review","input":"Synthetic operation"}`
	if response, body := messageRequest(t, f, "POST", "/v1/responses", raw, h); response.StatusCode != 200 {
		t.Fatal(response.StatusCode, body)
	}
	if child := readObservation(); child.account != parent.account || child.cache == parent.cache {
		t.Fatal("HTTP review did not retain the parent account and its own session", parent, child)
	}
	pool.ModelPolicy = &groups.ModelPolicy{Restricted: true, Models: []string{"synthetic-model"}}
	if _, err := f.groups.Save(ctx, 1, pool); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteJSON(frame); err != nil {
		t.Fatal(err)
	}
	rejected := readEvent("error")
	encoded, _ := json.Marshal(rejected)
	if !strings.Contains(string(encoded), "model_not_allowed") || len(observed) != 0 {
		t.Fatal("parent affinity bypassed per-turn group policy", rejected)
	}
	pool.ModelPolicy = &groups.ModelPolicy{Models: []string{}}
	if _, err := f.groups.Save(ctx, 1, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := f.keys.Revoke(ctx, f.userID, f.keyID); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteJSON(frame); err != nil {
		t.Fatal(err)
	}
	rejected = readEvent("error")
	encoded, _ = json.Marshal(rejected)
	if !strings.Contains(string(encoded), "invalid_api_key") || len(observed) != 0 {
		t.Fatal("parent affinity bypassed per-turn key revocation", rejected)
	}
}
