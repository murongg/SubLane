package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
)

func TestPersonalKeyOwnershipAndGatewayBoundary(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "key-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	identity, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	h := New(Options{Auth: identity, Keys: apikey.New(db), Ping: db.PingContext, StartedAt: time.Now()})
	origin := "http://example.test"
	setup := request(h, "POST", "/api/auth/setup", origin, map[string]string{"username": "owner-test", "password": "owner pass 42"}, nil)
	owner := setup.Result().Cookies()[0]
	memberInput := map[string]string{"username": "member-test", "password": "member pass 42"}
	if got := request(h, "POST", "/api/members", origin, memberInput, owner).Code; got != 201 {
		t.Fatalf("member create: %d", got)
	}
	login := request(h, "POST", "/api/auth/login", origin, memberInput, nil)
	member := login.Result().Cookies()[0]
	created := request(h, "POST", "/api/keys", origin, map[string]string{"name": "Synthetic laptop"}, member)
	if created.Code != 201 {
		t.Fatalf("member key create: %d", created.Code)
	}
	var key apikey.CreatedKey
	if err := json.Unmarshal(created.Body.Bytes(), &key); err != nil || key.Secret == "" {
		t.Fatal("missing creation secret")
	}
	list := request(h, "GET", "/api/keys", "", nil, member)
	if list.Code != 200 || strings.Contains(list.Body.String(), key.Secret) {
		t.Fatal("key listing exposed secret or denied owner")
	}
	if got := request(h, "POST", fmt.Sprintf("/api/keys/%d/revoke", key.Key.ID), origin, map[string]string{}, owner).Code; got != 404 {
		t.Fatalf("cross-user revocation: %d", got)
	}
	if got := request(h, "POST", "/api/keys", origin, map[string]any{"name": "Forged owner", "user_id": 1}, member).Code; got != 400 {
		t.Fatal("owner injection accepted")
	}
	gateway := func(path, token string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://example.test"+path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if got := gateway("/v1/models", "", member).Code; got != 401 {
		t.Fatal("browser session used as gateway key")
	}
	if got := gateway("/api/system", key.Secret, nil).Code; got != 401 {
		t.Fatal("API key granted administrator access")
	}
	if got := gateway("/api/keys", key.Secret, nil).Code; got != 401 {
		t.Fatal("API key accessed key management")
	}
	result := gateway("/v1/models", key.Secret, nil)
	if result.Code != 501 || !strings.Contains(result.Body.String(), "gateway_not_configured") {
		t.Fatalf("gateway readiness: %d", result.Code)
	}
	if got := request(h, "POST", fmt.Sprintf("/api/keys/%d/revoke", key.Key.ID), origin, map[string]string{}, member).Code; got != 200 {
		t.Fatal("revoke failed")
	}
	if got := gateway("/v1/models", key.Secret, nil).Code; got != 401 {
		t.Fatal("revoked gateway key accepted")
	}
}
