package server

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestMemberPoliciesProtectHTTPAndWebSocketTurns(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic\",\"output\":[]}}\n\n")
	})
	h := f.server.Config.Handler
	session, err := f.identity.Login(context.Background(), "owner-test", "owner pass 42")
	if err != nil {
		t.Fatal(err)
	}
	owner := &http.Cookie{Name: sessionCookie, Value: session.Token}
	memberLogin, err := f.identity.Login(context.Background(), "member-test", "member pass 42")
	if err != nil {
		t.Fatal(err)
	}
	member := &http.Cookie{Name: sessionCookie, Value: memberLogin.Token}
	path := "/api/members/" + strconv.FormatInt(f.userID, 10) + "/limits"
	policy := map[string]int{"requests_per_minute": 1, "max_concurrency": 1}
	if got := request(h, "PATCH", path, "http://example.test", policy, member).Code; got != 403 {
		t.Fatal("member changed limits", got)
	}
	if got := request(h, "PATCH", path, "http://foreign.example.test", policy, owner).Code; got != 403 {
		t.Fatal("foreign origin changed limits", got)
	}
	if got := request(h, "PATCH", path, "http://example.test", policy, owner).Code; got != 200 {
		t.Fatal("limits unavailable", got)
	}
	if got := request(h, "GET", "/api/me/limits", "", nil, member).Code; got != 200 {
		t.Fatal("own limits unavailable", got)
	}
	raw := `{"model":"synthetic-model","input":"synthetic"}`
	call := func() *http.Response {
		r, _ := http.NewRequest("POST", f.server.URL+"/v1/responses", strings.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+f.secret)
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	first := call()
	io.Copy(io.Discard, first.Body)
	first.Body.Close()
	if first.StatusCode != 200 {
		t.Fatal("first call failed", first.StatusCode)
	}
	second := call()
	body, _ := io.ReadAll(second.Body)
	second.Body.Close()
	if second.StatusCode != 429 || second.Header.Get("Retry-After") == "" || !strings.Contains(string(body), "member_rate_limited") {
		t.Fatal("HTTP quota not enforced", second.StatusCode, string(body))
	}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.server.URL, "http")+"/v1/responses", http.Header{"Authorization": {"Bearer " + f.secret}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"synthetic-model","input":[]}`)); err != nil {
		t.Fatal(err)
	}
	_, message, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(message), "member_rate_limited") {
		t.Fatal("WS bypassed shared quota", string(message), err)
	}
	for _, endpoint := range []string{"/api/me/usage?days=7", "/api/usage?days=7"} {
		want := 200
		if endpoint == "/api/usage?days=7" {
			want = 403
		}
		if got := request(h, "GET", endpoint, "", nil, member).Code; got != want {
			t.Fatal("usage role boundary", endpoint, got)
		}
	}
}
