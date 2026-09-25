package server

import (
	"encoding/json"
	"fmt"
	"github.com/murongg/SubLane/internal/auth"
	"strings"
	"testing"
)

func TestMemberHTTPAuthorizationAndRevocation(t *testing.T) {
	h := authFixture(t, "")
	origin := "http://example.test"
	setup := request(h, "POST", "/api/auth/setup", origin, map[string]string{"username": "owner-test", "password": "fake password 42", "workspace_name": "Synthetic workspace"}, nil)
	if setup.Code != 201 {
		t.Fatal(setup.Body.String())
	}
	owner := setup.Result().Cookies()[0]
	input := map[string]string{"username": "member-test", "password": "member pass 42"}
	if got := request(h, "POST", "/api/members", origin, input, nil).Code; got != 401 {
		t.Fatalf("anonymous member creation: %d", got)
	}
	created := request(h, "POST", "/api/members", origin, input, owner)
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var member auth.Member
	if err := json.Unmarshal(created.Body.Bytes(), &member); err != nil || member.Role != "member" {
		t.Fatalf("member role missing: %v", err)
	}
	if strings.Contains(created.Body.String(), "password") {
		t.Fatal("credentials exposed")
	}
	login := request(h, "POST", "/api/auth/login", origin, input, nil)
	if login.Code != 200 || !strings.Contains(login.Body.String(), `"role":"member"`) {
		t.Fatalf("member login: %d %s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	for _, path := range []string{"/api/system", "/api/members", "/api/future-management"} {
		if got := request(h, "GET", path, "", nil, cookie).Code; got != 403 {
			t.Fatalf("member accessed %s: %d", path, got)
		}
	}
	encodedPath := "/api/members/"
	for _, digit := range fmt.Sprint(member.ID) {
		encodedPath += fmt.Sprintf("%%%02X", digit)
	}
	if got := request(h, "PATCH", encodedPath, origin, map[string]bool{"enabled": true}, owner).Code; got != 200 {
		t.Fatalf("encoded member ID: %d", got)
	}
	path := fmt.Sprintf("/api/members/%d", member.ID)
	if got := request(h, "PATCH", path, origin, map[string]bool{"enabled": false}, cookie).Code; got != 403 {
		t.Fatalf("member can change status: %d", got)
	}
	if got := request(h, "POST", "/api/members", origin, input, cookie).Code; got != 403 {
		t.Fatalf("member can create accounts: %d", got)
	}
	state := request(h, "GET", "/api/auth/state", "", nil, cookie)
	if state.Code != 200 || !strings.Contains(state.Body.String(), `"role":"member"`) {
		t.Fatal("member cannot read own session")
	}
	if got := request(h, "PATCH", path, origin, map[string]bool{"enabled": false}, owner).Code; got != 200 {
		t.Fatalf("disable failed: %d", got)
	}
	if got := request(h, "GET", "/api/system", "", nil, cookie).Code; got != 401 {
		t.Fatalf("disabled session still authenticates: %d", got)
	}
	if result := request(h, "POST", "/api/auth/login", origin, input, nil); result.Code != 200 || !strings.Contains(result.Body.String(), `"needs_workspace":true`) {
		t.Fatalf("disabled workspace member did not reach the workspace chooser: %d %s", result.Code, result.Body.String())
	}
	if got := request(h, "PATCH", path, origin, map[string]bool{"enabled": true}, owner).Code; got != 200 {
		t.Fatal("enable failed")
	}
	if got := request(h, "POST", "/api/auth/login", origin, input, nil).Code; got != 200 {
		t.Fatalf("member cannot log in after enable: %d", got)
	}
	if got := request(h, "GET", "/api/system", "", nil, owner).Code; got != 200 {
		t.Fatal("member lifecycle damaged owner")
	}
}

func TestMemberHTTPValidationAndOwnerProtection(t *testing.T) {
	h := authFixture(t, "")
	origin := "http://example.test"
	setup := request(h, "POST", "/api/auth/setup", origin, map[string]string{"username": "owner-test", "password": "fake password 42", "workspace_name": "Synthetic workspace"}, nil)
	cookie := setup.Result().Cookies()[0]
	forged := map[string]string{"username": "member-test", "password": "member pass 42", "role": "admin"}
	if got := request(h, "POST", "/api/members", origin, forged, cookie).Code; got != 400 {
		t.Fatalf("role injection accepted: %d", got)
	}
	input := map[string]string{"username": "member-test", "password": "member pass 42"}
	if got := request(h, "POST", "/api/members", "https://evil.example.test", input, cookie).Code; got != 403 {
		t.Fatal("origin guard missing")
	}
	if got := request(h, "POST", "/api/members", origin, input, cookie).Code; got != 201 {
		t.Fatalf("creation failed: %d", got)
	}
	if got := request(h, "POST", "/api/members", origin, input, cookie).Code; got != 409 {
		t.Fatalf("duplicate accepted: %d", got)
	}
	if got := request(h, "PATCH", "/api/members/1", origin, map[string]bool{"enabled": false}, cookie).Code; got != 404 {
		t.Fatalf("owner mutation allowed: %d", got)
	}
	for _, path := range []string{"/api/members?cursor=-1", "/api/members?cursor=abc"} {
		if got := request(h, "GET", path, "", nil, cookie).Code; got != 400 {
			t.Fatalf("invalid cursor accepted: %d", got)
		}
	}
	for _, path := range []string{"/api/members/%2532", "/api/members/2%2Fextra"} {
		if got := request(h, "PATCH", path, origin, map[string]bool{"enabled": true}, cookie).Code; got != 400 {
			t.Fatalf("invalid encoded ID %s: %d", path, got)
		}
	}
	for _, body := range []any{map[string]any{}, map[string]string{"enabled": "false"}, map[string]any{"enabled": true, "role": "admin"}} {
		if got := request(h, "PATCH", "/api/members/2", origin, body, cookie).Code; got != 400 {
			t.Fatalf("invalid status body accepted: %d", got)
		}
	}
	list := request(h, "GET", "/api/members", "", nil, cookie)
	if list.Code != 200 || strings.Contains(list.Body.String(), "password") || !strings.Contains(list.Body.String(), `"username":"owner-test","role":"owner"`) {
		t.Fatalf("bad list: %s", list.Body.String())
	}
}
