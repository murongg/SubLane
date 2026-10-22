package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/groups"
)

func TestHiddenCodexModelsForwardWithoutListingOrBypassingPolicy(t *testing.T) {
	forwarded := 0
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			io.WriteString(w, `{"models":[{"slug":"synthetic-model","visibility":"list"},{"slug":"codex-auto-review","visibility":"hide"},{"slug":"synthetic-hidden","visibility":"hidden"}]}`)
			return
		}
		var input struct{ Model string }
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || (input.Model != "codex-auto-review" && input.Model != "synthetic-hidden") {
			t.Error("hidden model name changed", input.Model, err)
		}
		forwarded++
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_synthetic\",\"output\":[]}}\n\n")
	})
	ctx := context.Background()
	rows, err := f.accounts.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	accountID := rows[0].ID
	snapshot, err := f.accounts.Catalog(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.accounts.SaveCatalog(ctx, accountID, snapshot.Revision, snapshot.Models, time.Now().Unix(), "codex:legacy"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.forwarding.AccountCatalog(ctx, accountID, true); err != nil {
		t.Fatal(err)
	}
	h := f.server.Config.Handler
	login := request(h, "POST", "/api/auth/login", "http://example.test", map[string]string{"username": "owner-test", "password": "owner pass 42"}, nil)
	if login.Code != 200 {
		t.Fatal("administrator login", login.Code)
	}
	admin := login.Result().Cookies()[0]
	memberLogin := request(h, "POST", "/api/auth/login", "http://example.test", map[string]string{"username": "member-test", "password": "member pass 42"}, nil)
	if memberLogin.Code != 200 {
		t.Fatal("member login", memberLogin.Code)
	}
	member := memberLogin.Result().Cookies()[0]
	invoke := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+f.secret)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	assertListings := func(hidden ...string) {
		t.Helper()
		response := invoke("GET", "/v1/models", "")
		if response.Code != 200 || strings.Contains(response.Body.String(), "codex-auto-review") || strings.Contains(response.Body.String(), "synthetic-hidden") {
			t.Fatal("hidden capability entered public discovery", response.Code, response.Body.String())
		}
		assertHiddenCatalog(t, request(h, "GET", "/api/accounts/"+accountID+"/models", "", nil, admin), "codex-auto-review", "synthetic-hidden")
		assertHiddenCatalog(t, request(h, "GET", "/api/groups/1/models", "", nil, admin), hidden...)
		assertHiddenCatalog(t, request(h, "GET", fmt.Sprintf("/api/keys/%d/models", f.keyID), "", nil, member), hidden...)
	}
	assertListings("codex-auto-review", "synthetic-hidden")
	for _, model := range []string{"codex-auto-review", "synthetic-hidden"} {
		if response := invoke("POST", "/v1/responses", fmt.Sprintf(`{"model":%q,"input":"Synthetic operation"}`, model)); response.Code != 200 {
			t.Fatal("hidden model rejected", response.Code, response.Body.String())
		}
	}
	input := groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{accountID}, ModelPolicy: &groups.ModelPolicy{Restricted: true, Models: []string{"synthetic-model"}}}
	if _, err := f.groups.Save(ctx, 1, input); err != nil {
		t.Fatal(err)
	}
	assertListings()
	raw := `{"model":"codex-auto-review","input":"Synthetic operation"}`
	if response := invoke("POST", "/v1/responses", raw); response.Code != 403 || !strings.Contains(response.Body.String(), "model_not_allowed") || forwarded != 2 {
		t.Fatal("hidden model bypassed group policy", response.Code, response.Body.String(), forwarded)
	}
	input.ModelPolicy.Models = []string{"codex-auto-review"}
	if _, err := f.groups.Save(ctx, 1, input); err != nil {
		t.Fatal(err)
	}
	assertListings("codex-auto-review")
	if response := invoke("POST", "/v1/responses", raw); response.Code != 200 || forwarded != 3 {
		t.Fatal("explicit hidden model permission was ignored", response.Code, response.Body.String(), forwarded)
	}
	if err := f.groups.SetMemberGroups(ctx, f.userID, []int64{}); err != nil {
		t.Fatal(err)
	}
	if response := invoke("POST", "/v1/responses", raw); response.Code != 401 || forwarded != 3 {
		t.Fatal("hidden model bypassed revoked pool access", response.Code, response.Body.String(), forwarded)
	}
	if err := f.groups.SetMemberGroups(ctx, f.userID, []int64{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.keys.Update(ctx, f.userID, f.keyID, apikey.UpdateInput{Name: "Synthetic client", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if response := invoke("POST", "/v1/responses", raw); response.Code != 401 || forwarded != 3 {
		t.Fatal("hidden model bypassed paused key", response.Code, response.Body.String(), forwarded)
	}
}

func TestCatalogEndpointsRespectRoleOwnershipAndKeyLifecycle(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"models":[{"slug":"synthetic-model"}]}`)
	})
	h := f.server.Config.Handler
	login := func(name, password string) *http.Cookie {
		response := request(h, "POST", "/api/auth/login", "http://example.test", map[string]string{"username": name, "password": password}, nil)
		if response.Code != 200 {
			t.Fatal(response.Code)
		}
		return response.Result().Cookies()[0]
	}
	member := login("member-test", "member pass 42")
	admin := login("owner-test", "owner pass 42")
	path := fmt.Sprintf("/api/keys/%d/models", f.keyID)
	response := request(h, "GET", path, "", nil, member)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"id":"synthetic-model"`) || strings.Contains(response.Body.String(), "codex/") {
		t.Fatal("personal catalog", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "synthetic-upstream") || strings.Contains(response.Body.String(), f.secret) {
		t.Fatal("private account data leaked")
	}
	if result := request(h, "GET", path, "", nil, admin); result.Code != 404 {
		t.Fatal("admin read another owner's catalog", result.Code)
	}
	if result := request(h, "GET", "/api/groups/1/models", "", nil, member); result.Code != 403 {
		t.Fatal("member read administrator catalog", result.Code)
	}
	if result := request(h, "GET", "/api/groups/1/models", "", nil, admin); result.Code != 200 {
		t.Fatal("administrator catalog", result.Code)
	}
	rows, err := f.accounts.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	accountPath := "/api/accounts/" + rows[0].ID + "/models"
	if result := request(h, "GET", accountPath, "", nil, member); result.Code != 403 {
		t.Fatal("member read account catalog", result.Code)
	}
	if result := request(h, "GET", accountPath, "", nil, admin); result.Code != 200 {
		t.Fatal("account catalog", result.Code)
	}
	if result := request(h, "POST", accountPath+"/refresh", "https://foreign.example.test", map[string]any{}, admin); result.Code != 403 {
		t.Fatal("refresh lost origin protection", result.Code)
	}
	if _, err := f.keys.Update(context.Background(), f.userID, f.keyID, apikey.UpdateInput{Name: "Synthetic client", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if result := request(h, "GET", path, "", nil, member); result.Code != 409 {
		t.Fatal("paused key retained catalog access", result.Code)
	}
}

func TestImportStartsModelDiscoveryWithoutFailingCredentialStorage(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"models":[{"slug":"synthetic-discovered"}]}`)
	})
	h := f.server.Config.Handler
	login := request(h, "POST", "/api/auth/login", "http://example.test", map[string]string{"username": "owner-test", "password": "owner pass 42"}, nil)
	if login.Code != 200 {
		t.Fatal("login", login.Code, login.Body.String())
	}
	admin := login.Result().Cookies()[0]
	credential := fmt.Sprintf(`{"type":"codex","access_token":"synthetic-new-access","refresh_token":"synthetic-refresh","account_id":"synthetic-new","expired":"%s"}`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	result := request(h, "POST", "/api/accounts/import", "http://example.test", map[string]string{"provider": "codex", "name": "Synthetic auto-discovery", "auth_json": credential}, admin)
	if result.Code != 201 {
		t.Fatal(result.Code, result.Body.String())
	}
	var created accounts.Account
	if err := json.Unmarshal(result.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		catalog, err := f.accounts.Catalog(context.Background(), created.ID)
		if err == nil && catalog.UpdatedAt > 0 && len(catalog.Models) == 1 && catalog.Models[0] == "synthetic-discovered" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("import did not discover models")
}

func TestCachedModelsDoNotConsumeUpstreamRequestSlots(t *testing.T) {
	f := newForwardFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("fresh catalog contacted upstream") })
	for range f.forwarding.Policy().Capacity {
		release, err := f.forwarding.Acquire()
		if err != nil {
			t.Fatal(err)
		}
		defer release()
	}
	r := httptest.NewRequest(http.MethodGet, "http://example.test/v1/models", nil)
	r.Header.Set("Authorization", "Bearer "+f.secret)
	w := httptest.NewRecorder()
	f.server.Config.Handler.ServeHTTP(w, r)
	var catalog struct {
		Object string `json:"object"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &catalog) != nil || catalog.Object != "list" || len(catalog.Data) != 1 || catalog.Data[0].ID != "synthetic-model" {
		t.Fatal("cached native model listing", w.Code, w.Body.String())
	}
}

func assertHiddenCatalog(t *testing.T, response *httptest.ResponseRecorder, hidden ...string) {
	t.Helper()
	var view struct {
		Models       json.RawMessage `json:"models"`
		HiddenModels []string        `json:"hidden_models"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &view) != nil || !slices.Equal(view.HiddenModels, hidden) {
		t.Fatal("management catalog lost permitted hidden metadata", response.Code, response.Body.String(), hidden)
	}
	for _, id := range hidden {
		if strings.Contains(string(view.Models), id) {
			t.Fatal("hidden model entered visible model choices", string(view.Models))
		}
	}
}
