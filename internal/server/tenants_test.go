package server

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/gateway"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/tenants"
)

func TestWorkspaceHTTPRequiresLoginAndListsOwnedWorkspaces(t *testing.T) {
	connection, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	login, err := auth.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	h := New(Options{Auth: login, Tenants: tenants.New(connection), Ping: connection.PingContext, StartedAt: time.Now()})
	if got := request(h, http.MethodGet, "/api/tenants", "", nil, nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("anonymous workspace list: %d", got)
	}
	setup := request(h, http.MethodPost, "/api/auth/setup", "http://example.test", map[string]string{"username": "owner-test", "password": "fake password 42", "workspace_name": "Synthetic workspace"}, nil)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", setup.Code, setup.Body.String())
	}
	cookie := setup.Result().Cookies()[0]
	created := request(h, http.MethodPost, "/api/tenants", "http://example.test", map[string]string{"name": "Synthetic workspace"}, cookie)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"name":"Synthetic workspace"`) {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	listed := request(h, http.MethodGet, "/api/tenants", "", nil, cookie)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"name":"Synthetic workspace"`) {
		t.Fatalf("list: %d %s", listed.Code, listed.Body.String())
	}
	if _, err := connection.Exec("INSERT INTO users(id,username,role,password_hash,enabled,created_at) VALUES(2,'member-test','member','synthetic-hash',1,1)"); err != nil {
		t.Fatal(err)
	}
	added := request(h, http.MethodPost, "/api/tenants/1/members", "http://example.test", map[string]any{"user_id": 2, "role": "member"}, cookie)
	if added.Code != http.StatusNoContent {
		t.Fatalf("add member: %d %s", added.Code, added.Body.String())
	}
	member, ok, err := tenants.New(connection).Membership(context.Background(), 1, 2)
	if err != nil || !ok || member.Role != tenants.RoleMember {
		t.Fatalf("membership after API write: %+v %v %v", member, ok, err)
	}
}

func TestWorkspaceRoleComesFromMembership(t *testing.T) {
	ctx := context.Background()
	connection, err := storage.Open(ctx, filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	login, err := auth.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	platformOwner, err := login.Setup(ctx, "platform-owner", "synthetic password", "Synthetic workspace")
	if err != nil {
		t.Fatal(err)
	}
	member, err := login.CreateMember(ctx, "workspace-owner", "synthetic password")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := tenants.New(connection).Create(ctx, member.ID, "Second workspace")
	if err != nil {
		t.Fatal(err)
	}
	h := New(Options{Auth: login, Tenants: tenants.New(connection), TenantID: workspace.ID, Ping: connection.PingContext, StartedAt: time.Now()})
	session, err := login.Login(ctx, member.Username, "synthetic password")
	if err != nil {
		t.Fatal(err)
	}
	ownerCookie := &http.Cookie{Name: sessionCookie, Value: session.Token}
	if got := request(h, http.MethodGet, "/api/system", "", nil, ownerCookie).Code; got != http.StatusOK {
		t.Fatalf("workspace owner lacked admin access: %d", got)
	}
	if state := request(h, http.MethodGet, "/api/auth/state", "", nil, ownerCookie); !strings.Contains(state.Body.String(), `"role":"admin"`) || !strings.Contains(state.Body.String(), `"workspace_count":2`) {
		t.Fatalf("browser state did not use workspace role: %s", state.Body.String())
	}
	platformCookie := &http.Cookie{Name: sessionCookie, Value: platformOwner.Token}
	if got := request(h, http.MethodGet, "/api/system", "", nil, platformCookie).Code; got != http.StatusUnauthorized && got != http.StatusForbidden {
		t.Fatalf("administrator without workspace membership accessed it: %d", got)
	}
	if state := request(h, http.MethodGet, "/api/auth/state", "", nil, platformCookie); !strings.Contains(state.Body.String(), `"needs_workspace":true`) {
		t.Fatalf("session lost the workspace switching path: %s", state.Body.String())
	}
	wrongWorkspaceLogin := request(h, http.MethodPost, "/api/auth/login", "http://example.test",
		map[string]string{"username": "platform-owner", "password": "synthetic password"}, nil)
	if wrongWorkspaceLogin.Code != http.StatusOK || !strings.Contains(wrongWorkspaceLogin.Body.String(), `"needs_workspace":true`) || len(wrongWorkspaceLogin.Result().Cookies()) == 0 {
		t.Fatalf("global login could not select another workspace: %d %s", wrongWorkspaceLogin.Code, wrongWorkspaceLogin.Body.String())
	}
	if err := tenants.New(connection).Suspend(ctx, workspace.ID); err != nil {
		t.Fatal(err)
	}
	if got := request(h, http.MethodGet, "/api/system", "", nil, ownerCookie).Code; got != http.StatusUnauthorized {
		t.Fatalf("suspended workspace still authorized a browser session: %d", got)
	}
	if err := tenants.New(connection).AddMember(ctx, platformOwner.User.ID, 1, member.ID, tenants.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	platformHandler := New(Options{Auth: login, Tenants: tenants.New(connection), TenantID: 1})
	for _, path := range []string{"/api/settings/backup", "/api/settings/codex"} {
		if got := request(platformHandler, http.MethodGet, path, "", nil, ownerCookie).Code; got != http.StatusForbidden {
			t.Fatalf("tenant administrator reached platform settings %s: %d", path, got)
		}
	}
}

func TestPlatformOwnerCanHaveOrdinaryMemberLimitsInAnotherWorkspace(t *testing.T) {
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
	if _, err := identity.Setup(ctx, "synthetic-platform-owner", "synthetic password", "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	owner, err := identity.CreateMember(ctx, "synthetic-workspace-owner", "synthetic password")
	if err != nil {
		t.Fatal(err)
	}
	tenancy := tenants.New(connection)
	workspace, err := tenancy.Create(ctx, owner.ID, "Second workspace")
	if err != nil {
		t.Fatal(err)
	}
	if err := tenancy.AddMember(ctx, owner.ID, workspace.ID, 1, tenants.RoleMember); err != nil {
		t.Fatal(err)
	}
	forwarding := gateway.NewForTenant(ctx, connection, nil, nil, workspace.ID)
	defer forwarding.Close()
	h := New(Options{Auth: identity, Tenants: tenancy, TenantID: workspace.ID, Gateway: forwarding})
	session, err := identity.Login(ctx, owner.Username, "synthetic password")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookie, Value: session.Token}
	path := "/api/members/1/limits"
	if result := request(h, http.MethodPatch, path, "http://example.test", map[string]int64{
		"requests_per_minute": 10, "max_concurrency": 2,
	}, cookie); result.Code != http.StatusOK {
		t.Fatalf("member limits for global user 1: %d %s", result.Code, result.Body.String())
	}
	if result := request(h, http.MethodGet, path, "", nil, cookie); result.Code != http.StatusOK || !strings.Contains(result.Body.String(), `"requests_per_minute":10`) {
		t.Fatalf("read member limits for global user 1: %d %s", result.Code, result.Body.String())
	}
	if result := request(h, http.MethodPut, "/api/members/1/budgets", "http://example.test", map[string]any{
		"period": "day", "limit": 100, "enabled": true, "group_id": 0, "model": "",
	}, cookie); result.Code != http.StatusOK {
		t.Fatalf("member budget for global user 1: %d %s", result.Code, result.Body.String())
	}
}
