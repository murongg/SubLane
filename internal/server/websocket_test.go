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
)

func TestWebsocketPrewarmAndPerTurnKeyRevocation(t *testing.T) {
	var calls atomic.Int32
	fixture := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"output\":[]}}\n\n")
	})
	conn, response, err := websocket.DefaultDialer.Dial(strings.Replace(fixture.server.URL, "http://", "ws://", 1)+"/v1/responses", http.Header{"Authorization": {"Bearer " + fixture.secret}})
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("websocket upgrade: %d %v", status, err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	read := func(want string) map[string]any {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}
			var event map[string]any
			if json.Unmarshal(data, &event) != nil {
				t.Fatal("invalid websocket event")
			}
			if event["type"] == want {
				return event
			}
			if event["type"] == "error" {
				t.Fatal("unexpected websocket error")
			}
		}
	}
	if err := conn.WriteJSON(map[string]any{"type": "response.create", "model": "synthetic-model", "input": []any{}, "generate": false}); err != nil {
		t.Fatal(err)
	}
	warmed := read("response.completed")
	if calls.Load() != 0 {
		t.Fatal("prewarm made a generation request")
	}
	previous := warmed["response"].(map[string]any)["id"]
	if err := conn.WriteJSON(map[string]any{"type": "response.create", "previous_response_id": previous, "input": []any{}}); err != nil {
		t.Fatal(err)
	}
	read("response.completed")
	if calls.Load() != 1 {
		t.Fatal("generation was not forwarded")
	}
	if _, err := fixture.keys.Revoke(context.Background(), fixture.userID, fixture.keyID); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteJSON(map[string]any{"type": "response.create", "previous_response_id": "resp_test", "input": []any{}}); err != nil {
		t.Fatal(err)
	}
	rejected := read("error")
	detail := rejected["error"].(map[string]any)
	if detail["code"] != "invalid_api_key" || calls.Load() != 1 {
		t.Fatal("revoked key started another turn")
	}
}
