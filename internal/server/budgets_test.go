package server

import (
	"context"
	"github.com/murongg/SubLane/internal/gateway"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestTokenBudgetsProtectHTTPAndWebSocketTurns(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n")
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
	path := "/api/members/" + strconv.FormatInt(f.userID, 10) + "/budgets"
	policy := map[string]any{"period": "day", "limit": 5, "enabled": true, "group_id": 0, "model": ""}
	if got := request(h, "PUT", path, "http://example.test", policy, member).Code; got != 403 {
		t.Fatal("member changed limits", got)
	}
	if got := request(h, "PUT", path, "http://foreign.example.test", policy, owner).Code; got != 403 {
		t.Fatal("foreign origin changed limits", got)
	}
	if got := request(h, "PUT", path, "http://example.test", policy, owner).Code; got != 200 {
		t.Fatal("limits unavailable", got)
	}
	if got := request(h, "GET", "/api/me/budgets", "", nil, member).Code; got != 200 {
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
	if second.StatusCode != 429 || second.Header.Get("Retry-After") == "" || !strings.Contains(string(body), "token_quota_exceeded") {
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
	if err != nil || !strings.Contains(string(message), "token_quota_exceeded") {
		t.Fatal("WS bypassed shared quota", string(message), err)
	}
	own := request(h, "GET", "/api/me/budgets?user_id=1", "", nil, member)
	if !strings.Contains(own.Body.String(), `"used":5`) {
		t.Fatal("owner read missing actual budget", own.Body.String())
	}
	if got := request(h, "GET", path, "", nil, member).Code; got != 403 {
		t.Fatal("member read admin budgets", got)
	}
	if got := request(h, "PUT", path, "http://example.test", map[string]any{"period": "day", "limit": 10}, owner).Code; got != 400 {
		t.Fatal("missing enabled accepted", got)
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

func TestBudgetSettlementAuthorizationAndAudit(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic\",\"output\":[]}}\n\n")
	})
	h := f.server.Config.Handler
	session, err := f.identity.Login(context.Background(), "owner-test", "owner pass 42")
	if err != nil {
		t.Fatal(err)
	}
	owner := &http.Cookie{Name: sessionCookie, Value: session.Token}
	memberSession, err := f.identity.Login(context.Background(), "member-test", "member pass 42")
	if err != nil {
		t.Fatal(err)
	}
	member := &http.Cookie{Name: sessionCookie, Value: memberSession.Token}
	path := "/api/members/" + strconv.FormatInt(f.userID, 10) + "/budgets"
	if got := request(h, "PUT", path, "http://example.test", map[string]any{"period": "day", "limit": 100, "enabled": true}, owner); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	x, err := f.forwarding.Open(context.Background(), f.userID, 1, []byte(`{"model":"synthetic-model","input":[]}`), nil, gateway.Responses)
	if err != nil {
		t.Fatal(err)
	}
	if err := x.Events(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	x.Body.Close()
	page, err := f.forwarding.Budgets(context.Background(), f.userID)
	if err != nil || len(page.Pending) != 1 {
		t.Fatal(page, err)
	}
	input := map[string]any{"request_id": page.Pending[0].RequestID, "tokens": 7}
	if got := request(h, "POST", path+"/settle", "http://example.test", input, member).Code; got != 403 {
		t.Fatal("member settled usage", got)
	}
	if got := request(h, "POST", path+"/settle", "http://foreign.example.test", input, owner).Code; got != 403 {
		t.Fatal("foreign origin settled usage", got)
	}
	if got := request(h, "POST", path+"/settle", "http://example.test", input, owner); got.Code != 204 {
		t.Fatal(got.Code, got.Body.String())
	}
	if got := request(h, "POST", path+"/settle", "http://example.test", input, owner).Code; got != 204 {
		t.Fatal("retry not idempotent", got)
	}
	page, err = f.forwarding.Budgets(context.Background(), f.userID)
	if err != nil || page.Rules[0].Used != 7 || len(page.Pending) != 0 {
		t.Fatal(page, err)
	}
	action, resource, id := auditTarget(httptest.NewRequest("POST", path+"/settle", nil))
	if action != "member.budget_settle" || resource != "member" || id != strconv.FormatInt(f.userID, 10) {
		t.Fatal("failed settlement not audit classified", action, resource, id)
	}
}
