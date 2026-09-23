package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/tenants"
	"github.com/murongg/SubLane/internal/vault"
)

func TestWorkspaceDispatchUsesKeyForGatewayAndSelectionForManagement(t *testing.T) {
	ctx := context.Background()
	connection, err := storage.Open(ctx, filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	identity, err := auth.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "synthetic-admin", "synthetic-password", "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	member, err := identity.CreateMember(ctx, "synthetic-member", "synthetic-password")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := tenants.New(connection).Create(ctx, member.ID, "Second workspace")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := groups.NewForTenant(connection, workspace.ID).Save(ctx, 0, groups.Input{Name: "Second pool", Enabled: true, AccountIDs: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := vault.Open(filepath.Join(t.TempDir(), "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	key, err := apikey.NewForTenant(connection, cipher, workspace.ID).CreateInGroup(ctx, member.ID, pool.ID, "Synthetic key")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewMulti(connection, identity, tenants.New(connection), "", func(id int64) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, id)
		})
	})
	unknownMethod := httptest.NewRecorder()
	handler.ServeHTTP(unknownMethod, httptest.NewRequest(http.MethodPut, "/api/workspaces", nil))
	if unknownMethod.Code != http.StatusUnauthorized {
		t.Fatalf("workspace method fallback was not protected: %d %s", unknownMethod.Code, unknownMethod.Body.String())
	}
	ownerSession, err := identity.Login(ctx, member.Username, "synthetic-password")
	if err != nil {
		t.Fatal(err)
	}
	unsupported := httptest.NewRequest(http.MethodPut, "/api/workspaces", nil)
	unsupported.AddCookie(&http.Cookie{Name: sessionCookie, Value: ownerSession.Token})
	unsupportedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unsupportedResponse, unsupported)
	if unsupportedResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("workspace method fallback lost 405: %d %s", unsupportedResponse.Code, unsupportedResponse.Body.String())
	}
	for _, check := range []struct {
		path, header, bearer string
		want                 string
	}{
		{"/api/groups", "2", "", "2"},
		{"/v1/models", "1", key.Secret, "2"},
		{"/v1beta/models", "1", key.Secret, "2"},
		{"/api/groups", "", "", "1"},
	} {
		req := httptest.NewRequest(http.MethodGet, check.path, nil)
		if check.header != "" {
			req.Header.Set("X-SubLane-Workspace", check.header)
		}
		if check.bearer != "" {
			req.Header.Set("Authorization", "Bearer "+check.bearer)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != 200 || w.Body.String() != check.want {
			t.Fatalf("dispatch %s: %d %s, want %s", check.path, w.Code, w.Body.String(), check.want)
		}
	}
	for _, check := range []struct {
		path, header, want string
	}{
		{"/v1/messages", "X-Api-Key", "2"},
		{"/v1beta/models", "X-Goog-Api-Key", "2"},
		{"/v1beta/models?key=" + key.Secret, "", "2"},
		{"/v1beta/models", "Authorization", "2"},
	} {
		req := httptest.NewRequest(http.MethodGet, check.path, nil)
		if check.header == "Authorization" {
			req.Header.Set(check.header, "Bearer "+key.Secret)
		} else if check.header != "" {
			req.Header.Set(check.header, key.Secret)
		}
		req.Header.Set("X-SubLane-Workspace", "1")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK || w.Body.String() != check.want {
			t.Fatalf("native key dispatch %s (%s): %d %s, want %s", check.path, check.header, w.Code, w.Body.String(), check.want)
		}
	}
	conflicting := httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	conflicting.Header.Set("Authorization", "Bearer invalid")
	conflicting.Header.Set("X-Goog-Api-Key", key.Secret)
	conflictResponse := httptest.NewRecorder()
	handler.ServeHTTP(conflictResponse, conflicting)
	if conflictResponse.Body.String() != "1" {
		t.Fatalf("conflicting native credentials selected a workspace: %s", conflictResponse.Body.String())
	}
	if err := tenants.New(connection).Suspend(ctx, workspace.ID); err != nil {
		t.Fatal(err)
	}
	session, err := identity.Login(ctx, member.Username, "synthetic-password")
	if err != nil {
		t.Fatal(err)
	}
	listRequest := httptest.NewRequest(http.MethodGet, "/api/workspaces", nil)
	listRequest.Header.Set("X-SubLane-Workspace", "2")
	listRequest.AddCookie(&http.Cookie{Name: sessionCookie, Value: session.Token})
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != 200 || !strings.Contains(listResponse.Body.String(), "Second workspace") {
		t.Fatalf("suspended selection blocked the global workspace list: %d %s", listResponse.Code, listResponse.Body.String())
	}
	createRequest := httptest.NewRequest(http.MethodPost, "http://example.test/api/workspaces", strings.NewReader(`{"name":"Third workspace"}`))
	createRequest.Header.Set("X-SubLane-Workspace", "2")
	createRequest.Header.Set("Origin", "http://example.test")
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.AddCookie(&http.Cookie{Name: sessionCookie, Value: session.Token})
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != 201 || !strings.Contains(createResponse.Body.String(), "Third workspace") {
		t.Fatalf("suspended selection blocked new workspace creation: %d %s", createResponse.Code, createResponse.Body.String())
	}
}

func TestFreshInstallationStillServesSetupAssets(t *testing.T) {
	ctx := context.Background()
	connection, err := storage.Open(ctx, filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	identity, err := auth.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	h := NewMulti(connection, identity, tenants.New(connection), "", func(id int64) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, id)
		})
	})
	for _, path := range []string{"/", "/healthz", "/api/auth/state"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != 200 || w.Body.String() != "1" {
			t.Fatalf("fresh setup path %s: %d %s", path, w.Code, w.Body.String())
		}
	}
}
