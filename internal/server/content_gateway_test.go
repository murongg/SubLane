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
	"github.com/murongg/SubLane/internal/content"
)

func TestContentHTTPRejectsWithoutDispatch(t *testing.T) {
	var calls atomic.Int32
	fixture := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	pattern := "SYNTHETIC_SECRET"
	if _, err := fixture.content.Update(context.Background(), content.Input{Mode: "block", Rules: []content.RuleInput{{Name: "Synthetic", Kind: "text", Pattern: &pattern, Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, raw string }{
		{"/v1/responses", `{"model":"synthetic-model","input":"SYNTHETIC_SECRET"}`},
		{"/v1/chat/completions", `{"model":"synthetic-model","messages":[{"role":"user","content":"SYNTHETIC_SECRET"}]}`},
		{"/v1/messages", `{"model":"synthetic-model","max_tokens":10,"messages":[{"role":"user","content":"SYNTHETIC_SECRET"}]}`},
		{"/v1beta/models/synthetic-model:generateContent", `{"contents":[{"parts":[{"text":"SYNTHETIC_SECRET"}]}]}`},
	} {
		req, _ := http.NewRequest("POST", fixture.server.URL+tc.path, strings.NewReader(tc.raw))
		req.Header.Set("Authorization", "Bearer "+fixture.secret)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 403 || !strings.Contains(string(raw), "content_policy_blocked") || strings.Contains(string(raw), pattern) || calls.Load() != 0 {
			t.Fatalf("%s: %d %s calls=%d", tc.path, resp.StatusCode, raw, calls.Load())
		}
	}
}

func TestContentWebsocketChecksRetainedContextAndPrewarm(t *testing.T) {
	var calls atomic.Int32
	fixture := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_synthetic\",\"output\":[]}}\n\n")
	})
	conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(fixture.server.URL, "http://", "ws://", 1)+"/v1/responses", http.Header{"Authorization": {"Bearer " + fixture.secret}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	read := func(want string) map[string]any {
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}
			var event map[string]any
			if json.Unmarshal(raw, &event) != nil {
				t.Fatal("invalid event")
			}
			if event["type"] == want {
				return event
			}
			if event["type"] == "error" {
				t.Fatal("unexpected error", event)
			}
		}
	}
	if err = conn.WriteJSON(map[string]any{"type": "response.create", "model": "synthetic-model", "input": "SYNTHETIC_SECRET", "generate": false}); err != nil {
		t.Fatal(err)
	}
	completed := read("response.completed")
	response := completed["response"].(map[string]any)
	previous := response["id"]
	pattern := "SYNTHETIC_SECRET"
	if _, err = fixture.content.Update(context.Background(), content.Input{Mode: "block", Rules: []content.RuleInput{{Name: "Synthetic", Kind: "text", Pattern: &pattern, Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	for _, generate := range []bool{true, false} {
		if err = conn.WriteJSON(map[string]any{"type": "response.create", "previous_response_id": previous, "input": []any{}, "generate": generate}); err != nil {
			t.Fatal(err)
		}
		event := read("error")
		raw, _ := json.Marshal(event)
		if !strings.Contains(string(raw), "content_policy_blocked") || strings.Contains(string(raw), pattern) || event["request_id"] == nil || calls.Load() != 0 {
			t.Fatalf("retained context passed: %s calls=%d", raw, calls.Load())
		}
	}
	// A rejected turn must not be accepted into the retained transcript.
	if err = conn.WriteJSON(map[string]any{"type": "response.create", "model": "synthetic-model", "input": []map[string]any{{"type": "message", "role": "assistant", "content": []map[string]string{{"type": "output_text", "text": "safe synthetic history"}}}, {"type": "message", "role": "user", "content": []map[string]string{{"type": "input_text", "text": "safe synthetic text"}}}}}); err != nil {
		t.Fatal(err)
	}
	read("response.completed")
	if calls.Load() != 1 {
		t.Fatal("clean replacement did not recover", calls.Load())
	}
}
