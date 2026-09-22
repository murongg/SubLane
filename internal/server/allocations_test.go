package server

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/upstream"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAllocationManagementRolesAndPersonalIsolation(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	owner, err := f.identity.Login(context.Background(), "owner-test", "owner pass 42")
	if err != nil {
		t.Fatal(err)
	}
	member, err := f.identity.Login(context.Background(), "member-test", "member pass 42")
	if err != nil {
		t.Fatal(err)
	}
	adminCookie := &http.Cookie{Name: sessionCookie, Value: owner.Token}
	memberCookie := &http.Cookie{Name: sessionCookie, Value: member.Token}
	h := f.server.Config.Handler
	body := map[string]any{"name": "Synthetic team", "enabled": true, "member_ids": []int64{f.userID}}
	if r := request(h, "POST", "/api/teams", "http://example.test", body, memberCookie); r.Code != 403 {
		t.Fatal("member management", r.Code)
	}
	if r := request(h, "POST", "/api/teams", "http://foreign.example.test", body, adminCookie); r.Code != 403 {
		t.Fatal("origin", r.Code)
	}
	if r := request(h, "POST", "/api/teams", "http://example.test", body, adminCookie); r.Code != 201 {
		t.Fatal("team create", r.Code, r.Body.String())
	}
	if r := request(h, "GET", "/api/me/allocations?user_id=1", "", nil, memberCookie); r.Code != 200 || r.Body.String() != "{\"schemes\":[]}\n" {
		t.Fatal("own scope", r.Code, r.Body.String())
	}
}

func TestSchemeKeysIsolateTeamsAcrossHTTPAndWebSocket(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n")
	})
	ctx := context.Background()
	original, err := f.accounts.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.accounts.Authorize(ctx, "Synthetic second", accounts.Credential{AccessToken: "synthetic-2", RefreshToken: "synthetic-refresh-2", AccountID: "synthetic-subscription-2", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.accounts.SaveCatalog(ctx, second.ID, 0, []string{"synthetic-model"}, time.Now().Unix(), upstream.CatalogSource("codex")); err != nil {
		t.Fatal(err)
	}
	if _, err = f.groups.Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	manager := f.forwarding.Allocations()
	secrets := []string{}
	schemeIDs := []int64{}
	for i, account := range []string{original[0].ID, second.ID} {
		name := fmt.Sprintf("Synthetic %d", i)
		pool, err := f.groups.Save(ctx, 0, groups.Input{Name: name, Enabled: true, AccountIDs: []string{account}})
		if err != nil {
			t.Fatal(err)
		}
		team, err := manager.SaveTeam(ctx, 0, allocations.TeamInput{Name: name, Enabled: true, MemberIDs: []int64{f.userID}})
		if err != nil {
			t.Fatal(err)
		}
		config := allocations.Config{Mode: "tokens", Period: "day", Members: []allocations.Share{{UserID: f.userID, Limit: 5}}}
		if i == 1 {
			config.Mode = "amount"
			config.Members[0].Limit = 18
			config.Rates = []allocations.Rate{{Model: "synthetic-model", Input: 2000000, Cached: 1000000, Output: 6000000}}
		}
		scheme, err := manager.SaveScheme(ctx, 0, allocations.SchemeInput{Name: name, TeamID: team.ID, GroupID: pool.ID, Enabled: true, Config: config})
		if err != nil {
			t.Fatal(err)
		}
		key, err := f.keys.CreateInScheme(ctx, f.userID, scheme.ID, name, nil)
		if err != nil {
			t.Fatal(err)
		}
		secrets = append(secrets, key.Secret)
		schemeIDs = append(schemeIDs, scheme.ID)
	}
	call := func(secret string, want int) {
		t.Helper()
		req, _ := http.NewRequest("POST", f.server.URL+"/v1/responses", strings.NewReader(`{"model":"synthetic-model","input":[]}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+secret)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != want {
			t.Fatal(response.StatusCode, string(body))
		}
	}
	call(secrets[0], 200)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.server.URL, "http")+"/v1/responses", http.Header{"Authorization": {"Bearer " + secrets[0]}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"synthetic-model","input":[]}`)); err != nil {
		t.Fatal(err)
	}
	_, message, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(message), "allocation_exhausted") {
		t.Fatal("WS bypassed quota", string(message), err)
	}
	call(secrets[1], 200)
	duplicate, err := f.keys.CreateInScheme(ctx, f.userID, schemeIDs[1], "Synthetic duplicate", nil)
	if err != nil {
		t.Fatal(err)
	}
	call(duplicate.Secret, 429)
	page, err := manager.Own(ctx, f.userID)
	if err != nil || len(page) != 2 {
		t.Fatal(page, err)
	}
	if page[0].Balances[0].Used != 5 || page[1].Balances[0].Used != 18 {
		t.Fatal("ledgers mixed", page)
	}
}
