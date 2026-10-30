package demo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDemoInsightsShowSyntheticMetadataWithoutMemberAccountIdentities(t *testing.T) {
	instance, err := New(t.Context(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close() })
	call := func(path string, cookie *http.Cookie) []byte {
		r := httptest.NewRequest("GET", "http://demo.example.test"+path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		instance.Handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	login := func(username string) *http.Cookie {
		r := httptest.NewRequest("POST", "http://demo.example.test/api/auth/login", strings.NewReader(`{"username":"`+username+`","password":"sublane-demo"}`))
		r.Header.Set("Origin", "http://demo.example.test")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		instance.Handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK || len(w.Result().Cookies()) == 0 {
			t.Fatalf("login: %d %s", w.Code, w.Body.String())
		}
		return w.Result().Cookies()[0]
	}
	admin := login(Username)
	var resources struct{ Accounts []struct{ ID string } }
	if err := json.Unmarshal(call("/api/accounts", admin), &resources); err != nil || len(resources.Accounts) == 0 {
		t.Fatal("missing synthetic resources", err)
	}
	var catalog struct {
		Capabilities map[string]struct {
			ContextWindow int64 `json:"context_window"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(call("/api/accounts/"+resources.Accounts[0].ID+"/models", admin), &catalog); err != nil || catalog.Capabilities["demo-codex"].ContextWindow == 0 {
		t.Fatal("demo model capabilities missing", catalog, err)
	}
	for _, actor := range []struct {
		name string
		path string
	}{
		{Username, "/api/requests"},
		{"demo-developer", "/api/me/requests"},
	} {
		var page struct {
			Requests []struct {
				AccountID   string          `json:"account_id"`
				AccountName string          `json:"account_name"`
				Route       json.RawMessage `json:"route"`
			}
		}
		if err := json.Unmarshal(call(actor.path, login(actor.name)), &page); err != nil || len(page.Requests) == 0 {
			t.Fatal("demo request records missing", err)
		}
		for _, request := range page.Requests {
			if len(request.Route) == 0 || string(request.Route) == "null" {
				t.Fatal("demo route explanation missing")
			}
			if actor.name != Username && (request.AccountID != "" || request.AccountName != "") {
				t.Fatal("member history disclosed a subscription account")
			}
		}
	}
}
