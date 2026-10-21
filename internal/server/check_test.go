package server

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/upstream"
)

func TestModelCheckRequiresAdministratorAndSameOrigin(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"OK\"}]}]}}\n\n")
	})
	h := f.server.Config.Handler
	rows, _ := f.accounts.List(t.Context())
	path := "/api/accounts/" + rows[0].ID + "/models/check"
	input := map[string]string{"model": "synthetic-model"}
	login := func(name, password string) *http.Cookie {
		return request(h, "POST", "/api/auth/login", "http://example.test", map[string]string{"username": name, "password": password}, nil).Result().Cookies()[0]
	}
	owner := login("owner-test", "owner pass 42")
	member := login("member-test", "member pass 42")
	for _, tc := range []struct {
		cookie *http.Cookie
		origin string
		status int
	}{
		{nil, "http://example.test", 401},
		{member, "http://example.test", 403},
		{owner, "https://foreign.example.test", 403},
	} {
		if response := request(h, "POST", path, tc.origin, input, tc.cookie); response.Code != tc.status {
			t.Fatal("check authorization", response.Code, response.Body.String())
		}
	}
	if response := request(h, "POST", path, "http://example.test", map[string]string{"model": "invalid model"}, owner); response.Code != 400 {
		t.Fatal("model validation", response.Code, response.Body.String())
	}
	response := request(h, "POST", path, "http://example.test", input, owner)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"available":true`) || strings.Contains(response.Body.String(), "OK") || strings.Contains(response.Body.String(), "synthetic-upstream-access") {
		t.Fatal("model check", response.Code, response.Body.String())
	}
	if response := request(h, "POST", "/api/channels/"+rows[0].ID+"/models/check", "http://example.test", input, owner); response.Code != 404 {
		t.Fatal("channel check accepted a subscription", response.Code)
	}
}

func TestChannelModelCheckUsesChannelRoute(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer synthetic-channel-key" {
			t.Error("check did not use the channel")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"id\":\"synthetic-chat\",\"object\":\"chat.completion.chunk\",\"model\":\"synthetic-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"OK\"},\"finish_reason\":null}]}\n\n")
		io.WriteString(w, "data: {\"id\":\"synthetic-chat\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})
	channel, err := f.accounts.ImportProvider(t.Context(), "openai", "Synthetic channel", []byte(`{"api_key":"synthetic-channel-key","base_url":"https://relay.example.test/v1"}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.accounts.SaveCatalog(t.Context(), channel.ID, 0, []string{"synthetic-model"}, time.Now().Unix(), upstream.CatalogSource("openai")); err != nil {
		t.Fatal(err)
	}
	h := f.server.Config.Handler
	owner := request(h, "POST", "/api/auth/login", "http://example.test", map[string]string{"username": "owner-test", "password": "owner pass 42"}, nil).Result().Cookies()[0]
	response := request(h, "POST", "/api/channels/"+channel.ID+"/models/check", "http://example.test", map[string]string{"model": "synthetic-model"}, owner)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"available":true`) {
		t.Fatal("channel check", response.Code, response.Body.String())
	}
}
