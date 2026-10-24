package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/gateway"
)

func TestPersonalKeyStatisticsFilterEveryViewAndKeepRevokedUsage(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"input_tokens_details\":{\"cached_tokens\":1}}}}\n\n")
	})
	ctx := context.Background()
	second, err := f.keys.CreateInGroup(ctx, f.userID, 1, "Synthetic second client")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := f.keys.CreateInGroup(ctx, 1, 1, "Synthetic owner client")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{f.secret, f.secret, second.Secret, owner.Secret} {
		r, err := http.NewRequest("POST", f.server.URL+"/v1/responses", strings.NewReader(`{"model":"synthetic-model","input":"synthetic"}`))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatal("synthetic inference failed", response.StatusCode)
		}
	}
	if _, err := f.keys.Revoke(ctx, f.userID, second.Key.ID); err != nil {
		t.Fatal(err)
	}
	login, err := f.identity.Login(ctx, "member-test", "member pass 42")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookie, Value: login.Token}
	for _, test := range []struct {
		keyID, requests int64
	}{{f.keyID, 2}, {second.Key.ID, 1}, {0, 3}} {
		suffix := ""
		if test.keyID != 0 {
			suffix = fmt.Sprintf("&key_id=%d", test.keyID)
		}
		response := request(f.server.Config.Handler, "GET", "/api/me/usage?days=7"+suffix, "", nil, cookie)
		var page gateway.Statistics
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil {
			t.Fatal("key statistics unavailable", response.Code, response.Body.String())
		}
		if page.Totals.Requests != test.requests || page.Totals.InputTokens != 3*test.requests || page.Totals.OutputTokens != 2*test.requests || page.Totals.CachedTokens != test.requests {
			t.Fatalf("key %d totals escaped filter: %+v", test.keyID, page.Totals)
		}
		for _, rows := range [][]gateway.Statistic{page.Days, page.Models, page.Groups} {
			var requests, input int64
			for _, row := range rows {
				requests += row.Requests
				input += row.InputTokens
			}
			if requests != test.requests || input != 3*test.requests {
				t.Fatal("breakdown escaped key filter", rows)
			}
		}
		var requests, input int64
		for _, cell := range page.Activity.Cells {
			requests += cell.Requests
			input += cell.InputTokens
		}
		if requests != test.requests || input != 3*test.requests {
			t.Fatal("heatmap escaped key filter", requests, input)
		}
	}
	for _, test := range []struct {
		suffix string
		status int
	}{{"-1", 400}, {"0", 400}, {"invalid", 400}, {"9223372036854775808", 400}, {"%zz", 400}, {"1;ignored", 400}, {"1&key_id=2", 400}, {"", 400}, {fmt.Sprint(owner.Key.ID), 404}, {"999999", 404}} {
		response := request(f.server.Config.Handler, "GET", "/api/me/usage?days=7&key_id="+test.suffix, "", nil, cookie)
		if response.Code != test.status {
			t.Fatal("key filter validation/ownership", test.suffix, response.Code, response.Body.String())
		}
	}
	ownerLogin, err := f.identity.Login(ctx, "owner-test", "owner pass 42")
	if err != nil {
		t.Fatal(err)
	}
	for _, filter := range []string{"1", "%zz", "1;ignored"} {
		response := request(f.server.Config.Handler, "GET", "/api/usage?key_id="+filter, "", nil, &http.Cookie{Name: sessionCookie, Value: ownerLogin.Token})
		if response.Code != 400 {
			t.Fatal("team statistics accepted unsupported key filter", filter, response.Code)
		}
	}
}
