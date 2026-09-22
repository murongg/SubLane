package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/gateway"
)

func TestHourlyActivityHTTPUsesTheSessionIdentity(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic\",\"output\":[]}}\n\n")
	})
	ctx := context.Background()
	ownerKey, err := f.keys.CreateInGroup(ctx, 1, 1, "Synthetic owner client")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{f.secret, ownerKey.Secret, ownerKey.Secret} {
		request, _ := http.NewRequest("POST", f.server.URL+"/v1/responses", strings.NewReader(`{"model":"synthetic-model","input":"synthetic"}`))
		request.Header.Set("Authorization", "Bearer "+key)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatal("synthetic request failed", response.StatusCode)
		}
	}
	for _, test := range []struct {
		username, password, path string
		want                     int64
	}{
		{"member-test", "member pass 42", "/api/me/usage?days=1&user_id=1&scope=all", 1},
		{"owner-test", "owner pass 42", "/api/usage?days=1", 3},
	} {
		login, err := f.identity.Login(ctx, test.username, test.password)
		if err != nil {
			t.Fatal(err)
		}
		response := request(f.server.Config.Handler, "GET", test.path, "", nil, &http.Cookie{Name: sessionCookie, Value: login.Token})
		var page gateway.Statistics
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil {
			t.Fatal("usage unavailable", response.Code)
		}
		var count int64
		for _, cell := range page.Activity.Cells {
			count += cell.Requests
		}
		if count != test.want {
			t.Fatal("hourly ownership escaped session", test.username, count)
		}
	}
}
