package server

import (
	"context"
	"net/http"
	"strconv"
	"testing"
)

func TestPasswordHTTPPermissionsAndRevocation(t *testing.T) {
	f := newForwardFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected upstream call") })
	h := f.server.Config.Handler
	owner, err := f.identity.Login(context.Background(), "owner-test", "owner pass 42")
	if err != nil {
		t.Fatal(err)
	}
	member, err := f.identity.Login(context.Background(), "member-test", "member pass 42")
	if err != nil {
		t.Fatal(err)
	}
	ownerCookie := &http.Cookie{Name: sessionCookie, Value: owner.Token}
	memberCookie := &http.Cookie{Name: sessionCookie, Value: member.Token}
	body := map[string]string{"current_password": "member pass 42", "new_password": "synthetic-new-pass"}
	for _, origin := range []string{"", "http://foreign.example.test"} {
		if got := request(h, "POST", "/api/me/password", origin, body, memberCookie).Code; got != 403 {
			t.Fatal("cross-origin password change", got)
		}
	}
	path := "/api/members/" + strconv.FormatInt(member.User.ID, 10) + "/password"
	if got := request(h, "POST", path, "http://example.test", map[string]string{"new_password": "synthetic-new-pass"}, memberCookie).Code; got != 403 {
		t.Fatal("member reset another password", got)
	}
	if got := request(h, "POST", "/api/me/password", "http://example.test", body, memberCookie).Code; got != 200 {
		t.Fatal("change failed", got)
	}
	if got := request(h, "GET", "/api/keys", "", nil, memberCookie).Code; got != 401 {
		t.Fatal("old session alive", got)
	}
	if _, err := f.keys.Authenticate(context.Background(), f.secret); err != nil {
		t.Fatal("password change revoked independent gateway key", err)
	}
	if got := request(h, "POST", path, "http://example.test", map[string]string{"new_password": "synthetic-reset"}, ownerCookie).Code; got != 204 {
		t.Fatal("admin reset failed", got)
	}
	if got := request(h, "POST", "/api/members/1/password", "http://example.test", map[string]string{"new_password": "synthetic-reset"}, ownerCookie).Code; got != 404 {
		t.Fatal("member reset reached admin", got)
	}
}
