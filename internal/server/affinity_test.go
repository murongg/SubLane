package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/gateway"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/upstream"
)

func switchFixture(t *testing.T, supported bool) (forwardFixture, string, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_synthetic\",\"output\":[{\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"Synthetic reply\"}]}]}}\n\n")
	})
	ctx := context.Background()
	rows, err := f.accounts.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatal("synthetic account missing", err)
	}
	original := rows[0].ID
	models := []string{"synthetic-model"}
	if supported {
		models = append(models, "synthetic-next")
	}
	if err := f.accounts.SaveCatalog(ctx, original, 0, models, time.Now().Unix(), upstream.CatalogSource("codex")); err != nil {
		t.Fatal(err)
	}
	other, err := f.accounts.Authorize(ctx, "Synthetic second account", accounts.Credential{Provider: "codex", AccessToken: "synthetic-second-access", RefreshToken: "synthetic-second-refresh", AccountID: "synthetic-second-account", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.accounts.SaveCatalog(ctx, other.ID, 0, []string{"synthetic-next"}, time.Now().Unix(), upstream.CatalogSource("codex")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.groups.Save(ctx, 1, groups.Input{Name: "Synthetic pool", Enabled: true, AccountIDs: []string{original, other.ID}}); err != nil {
		t.Fatal(err)
	}
	return f, original, calls
}

func assertSwitchHistory(t *testing.T, f forwardFixture, account string, supported bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		page, err := f.forwarding.Requests(context.Background(), gateway.RequestFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total == 2 {
			for _, record := range page.Requests {
				if record.AccountID != account {
					t.Fatal("model switch moved the conversation account", record.AccountID)
				}
				if record.Model == "synthetic-next" && !supported && (record.ErrorCode != "conversation_model_conflict" || record.Outcome != "rejected" || record.UpstreamStatus != nil) {
					t.Fatal("model conflict was recorded as an account failure or sent upstream", record)
				}
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("model switch lost request history", page.Total)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestHTTPContinuationModelSwitch(t *testing.T) {
	for _, supported := range []bool{true, false} {
		t.Run(map[bool]string{true: "supported", false: "unavailable on bound account"}[supported], func(t *testing.T) {
			f, original, calls := switchFixture(t, supported)
			for i, body := range []string{
				`{"model":"synthetic-model","input":"Synthetic question"}`,
				`{"model":"synthetic-next","input":[{"type":"reasoning","encrypted_content":"synthetic-reasoning"},{"role":"user","content":"Synthetic follow-up"}]}`,
			} {
				r, err := http.NewRequest(http.MethodPost, f.server.URL+"/v1/responses", strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				r.Header.Set("Authorization", "Bearer "+f.secret)
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Session_id", "synthetic-http-model-switch")
				response, err := http.DefaultClient.Do(r)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if i == 1 && !supported {
					if response.StatusCode != http.StatusConflict || !strings.Contains(string(raw), `"code":"conversation_model_conflict"`) || !strings.Contains(string(raw), "new conversation") {
						t.Fatal("unsupported switch did not explain the model conflict", response.StatusCode, string(raw))
					}
				} else if response.StatusCode != http.StatusOK {
					t.Fatal("supported switch rejected", response.StatusCode, string(raw))
				}
			}
			want := int32(1)
			if supported {
				want = 2
			}
			if calls.Load() != want {
				t.Fatal("model switch made unexpected upstream calls", calls.Load())
			}
			assertSwitchHistory(t, f, original, supported)
		})
	}
}

func TestWebsocketContinuationModelSwitch(t *testing.T) {
	for _, supported := range []bool{true, false} {
		t.Run(map[bool]string{true: "supported", false: "unavailable on bound account"}[supported], func(t *testing.T) {
			f, original, calls := switchFixture(t, supported)
			conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(f.server.URL, "http://", "ws://", 1)+"/v1/responses", http.Header{"Authorization": {"Bearer " + f.secret}, "Session_id": {"synthetic-websocket-model-switch"}})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			for i, body := range []string{
				`{"type":"response.create","model":"synthetic-model","input":"Synthetic question"}`,
				`{"type":"response.create","model":"synthetic-next","previous_response_id":"resp_synthetic","input":"Synthetic follow-up"}`,
			} {
				if err := conn.WriteMessage(websocket.TextMessage, []byte(body)); err != nil {
					t.Fatal(err)
				}
				for {
					_, raw, err := conn.ReadMessage()
					if err != nil {
						t.Fatal(err)
					}
					var event struct {
						Type string `json:"type"`
					}
					if err := json.Unmarshal(raw, &event); err != nil {
						t.Fatal(err)
					}
					if event.Type != "error" && event.Type != "response.completed" {
						continue
					}
					if i == 1 && !supported {
						if event.Type != "error" || !strings.Contains(string(raw), `"code":"conversation_model_conflict"`) || !strings.Contains(string(raw), "new conversation") {
							t.Fatal("rebuilt history bypassed continuation identity or lost the model conflict", string(raw))
						}
					} else if event.Type == "error" {
						t.Fatal("supported websocket switch rejected", string(raw))
					}
					break
				}
			}
			if supported && calls.Load() != 2 || !supported && calls.Load() != 1 {
				t.Fatal("websocket switch made unexpected upstream calls", calls.Load())
			}
			assertSwitchHistory(t, f, original, supported)
		})
	}
}

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
