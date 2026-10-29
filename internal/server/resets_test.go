package server

import (
	"context"
	"encoding/json"
	"github.com/murongg/SubLane/internal/resets"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestResetManagementRequiresAdminOriginAndWorkspaceOwnership(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected upstream IO") })
	count := int64(2)
	calls := 0
	s := resets.New(f.connection, func() *time.Location { return time.UTC }, func(context.Context, int64, string) (resets.Observation, error) {
		return resets.Observation{Fresh: true, Blocked: true, Credits: &count}, nil
	}, func(context.Context, int64, string, string) (string, error) { calls++; return "reset", nil })
	h := New(Options{Auth: f.identity, Accounts: f.accounts, Keys: f.keys, Groups: f.groups, Gateway: f.forwarding, ResetCredits: s})
	list, err := f.accounts.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/accounts/" + list[0].ID + "/resets"
	origin := "http://example.test"
	owner := request(h, "POST", "/api/auth/login", origin, map[string]string{"username": "owner-test", "password": "owner pass 42"}, nil).Result().Cookies()[0]
	member := request(h, "POST", "/api/auth/login", origin, map[string]string{"username": "member-test", "password": "member pass 42"}, nil).Result().Cookies()[0]
	for _, method := range []string{"GET", "PUT", "POST"} {
		endpoint := path
		if method == "POST" {
			endpoint += "/consume"
		}
		if got := request(h, method, endpoint, origin, map[string]any{}, nil); got.Code != 401 {
			t.Fatal(got.Code)
		}
		if got := request(h, method, endpoint, origin, map[string]any{}, member); got.Code != 403 {
			t.Fatal(got.Code)
		}
	}
	settings := map[string]any{"scheduled": false, "exhausted": true, "frequency": "daily", "time": "09:00", "date": "", "weekday": 1, "daily_limit": 1}
	if got := request(h, "PUT", path, "https://foreign.example.test", settings, owner); got.Code != 403 {
		t.Fatal("origin bypass", got.Code)
	}
	if got := request(h, "PUT", path, origin, settings, owner); got.Code != 200 {
		t.Fatal(got.Code, got.Body)
	}
	if got := request(h, "GET", path, "", nil, owner); got.Code != 200 || !strings.Contains(got.Body.String(), `"exhausted":true`) {
		t.Fatal(got.Code, got.Body)
	}
	if got := request(h, "POST", path+"/consume", origin, map[string]any{}, owner); got.Code != 200 || calls != 1 {
		t.Fatal(got.Code, got.Body, calls)
	}
	if got := request(h, "GET", "/api/accounts/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/resets", "", nil, owner); got.Code != 404 {
		t.Fatal(got.Code)
	}
	got := request(h, "GET", path, "", nil, owner)
	var state resets.State
	if json.Unmarshal(got.Body.Bytes(), &state) != nil || state.UsedToday != 1 {
		t.Fatal(got.Body)
	}
}
