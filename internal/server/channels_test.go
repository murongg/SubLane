package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestChannelManagementSeparatesSubscriptionsAndSecrets(t *testing.T) {
	f := newForwardFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected provider IO") })
	h := New(Options{Auth: f.identity, Accounts: f.accounts})
	origin := "http://example.test"
	login := request(h, "POST", "/api/auth/login", origin, map[string]string{"username": "owner-test", "password": "owner pass 42"}, nil)
	owner := login.Result().Cookies()[0]
	input := map[string]string{"name": "Synthetic channel", "api_key": "synthetic-channel-key", "base_url": "https://relay.example.test/v1"}
	created := request(h, "POST", "/api/channels", origin, input, owner)
	if created.Code != 201 {
		t.Fatal("channel creation", created.Code, created.Body.String())
	}
	var channel struct{ ID, Name, Protocol, BaseURL, Status string }
	var fields map[string]json.RawMessage
	if json.Unmarshal(created.Body.Bytes(), &fields) != nil {
		t.Fatal("channel response")
	}
	json.Unmarshal(created.Body.Bytes(), &channel)
	if channel.ID == "" || channel.Protocol != "openai" || channel.Status != "unverified" {
		t.Fatal("channel management model", created.Body.String())
	}
	for _, field := range []string{"email", "plan", "expires_at", "api_key", "access_token"} {
		if _, ok := fields[field]; ok {
			t.Fatal("subscription/secret field in channel", field)
		}
	}
	listed := request(h, "GET", "/api/channels", "", nil, owner)
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), channel.ID) || strings.Contains(listed.Body.String(), "synthetic-channel-key") {
		t.Fatal("channel list", listed.Code, listed.Body.String())
	}
	subscriptions := request(h, "GET", "/api/accounts", "", nil, owner)
	if subscriptions.Code != 200 || strings.Contains(subscriptions.Body.String(), channel.ID) || !strings.Contains(subscriptions.Body.String(), "Synthetic subscription") {
		t.Fatal("channel leaked into subscription list", subscriptions.Body.String())
	}
	if result := request(h, "PATCH", "/api/channels/"+channel.ID, origin, map[string]bool{"enabled": false}, owner); result.Code != 200 {
		t.Fatal("channel disable", result.Code)
	}
	input["api_key"] = "synthetic-rotated-key"
	if result := request(h, "PUT", "/api/channels/"+channel.ID+"/key", origin, input, owner); result.Code != 200 || strings.Contains(result.Body.String(), "synthetic-rotated-key") {
		t.Fatal("channel rotation", result.Code, result.Body.String())
	}
	rows, _ := f.accounts.List(t.Context())
	for _, a := range rows {
		if a.Provider == "codex" {
			if result := request(h, "DELETE", "/api/channels/"+a.ID, origin, map[string]string{}, owner); result.Code != 404 {
				t.Fatal("subscription accepted as channel", result.Code)
			}
		}
	}
	if result := request(h, "GET", "/api/channels", "", nil, nil); result.Code != 401 {
		t.Fatal("anonymous channels", result.Code)
	}
	memberLogin := request(h, "POST", "/api/auth/login", origin, map[string]string{"username": "member-test", "password": "member pass 42"}, nil)
	member := memberLogin.Result().Cookies()[0]
	if result := request(h, "POST", "/api/channels", origin, input, member); result.Code != 403 {
		t.Fatal("member channel creation", result.Code)
	}
	if result := request(h, "POST", "/api/channels", "https://other.example.test", input, owner); result.Code != 403 {
		t.Fatal("cross-origin channel creation", result.Code)
	}
}
