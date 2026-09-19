package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/murongg/SubLane/internal/accounts"
)

func TestAccountUsageRequiresAdminAndRefreshesRejectedCredential(t *testing.T) {
	var calls, refreshes atomic.Int32
	fixture := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			refreshes.Add(1)
			io.WriteString(w, `{"access_token":"synthetic-rotated","expires_in":3600}`)
			return
		}
		if r.URL.Path != "/backend-api/wham/usage" {
			t.Errorf("wrong upstream: %s", r.URL.Path)
		}
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer synthetic-rotated" {
			w.WriteHeader(401)
			return
		}
		io.WriteString(w, `{"rate_limit":{"primary_window":{"used_percent":35,"limit_window_seconds":18000,"reset_at":2000000000}},"access_token":"synthetic-private"}`)
	})
	h := fixture.server.Config.Handler
	origin := "http://example.test"
	login := request(h, "POST", "/api/auth/login", origin, map[string]string{"username": "owner-test", "password": "owner pass 42"}, nil)
	owner := login.Result().Cookies()[0]
	var listed struct {
		Accounts []accounts.Account `json:"accounts"`
	}
	if err := json.Unmarshal(request(h, "GET", "/api/accounts", "", nil, owner).Body.Bytes(), &listed); err != nil || len(listed.Accounts) != 1 {
		t.Fatal("missing fixture account", err)
	}
	path := "/api/accounts/" + listed.Accounts[0].ID + "/usage"
	memberLogin := request(h, "POST", "/api/auth/login", origin, map[string]string{"username": "member-test", "password": "member pass 42"}, nil)
	member := memberLogin.Result().Cookies()[0]
	if request(h, "GET", path, "", nil, nil).Code != 401 || request(h, "GET", path, "", nil, member).Code != 403 || calls.Load() != 0 {
		t.Fatal("usage authorization failed")
	}
	result := request(h, "GET", path, "", nil, owner)
	if result.Code != 200 || calls.Load() != 2 || refreshes.Load() != 1 || !strings.Contains(result.Body.String(), `"used_percent":35`) || strings.Contains(result.Body.String(), "synthetic-private") || result.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("usage: status=%d calls=%d refresh=%d body=%s", result.Code, calls.Load(), refreshes.Load(), result.Body)
	}
	if result := request(h, "POST", path+"/refresh", origin, map[string]string{}, member); result.Code != 403 {
		t.Fatal("member refreshed quota")
	}
	if result := request(h, "POST", path+"/refresh", "https://other.example.test", map[string]string{}, owner); result.Code != 403 {
		t.Fatal("quota refresh lost origin protection")
	}
	if result := request(h, "POST", path+"/refresh", origin, map[string]string{}, owner); result.Code != 200 || calls.Load() != 2 || !strings.Contains(result.Body.String(), `"stale":false`) {
		t.Fatal("manual refresh did not honor cached cooldown", result.Code, result.Body)
	}
	request(h, "PATCH", "/api/accounts/"+listed.Accounts[0].ID, origin, map[string]bool{"enabled": false}, owner)
	if result := request(h, "GET", path, "", nil, owner); result.Code != 409 || calls.Load() != 2 {
		t.Fatal("disabled account queried upstream")
	}
}
