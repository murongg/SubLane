package gateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/groups"
)

func TestHiddenGrokCapabilitiesPersistAndKeepAdmissionChecks(t *testing.T) {
	ctx := context.Background()
	discoveries := 0
	s, _ := discoveryGateway(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "cli-chat-proxy.grok.com" || r.URL.Path != "/v1/models" {
			t.Fatal("unexpected discovery endpoint", r.URL)
		}
		discoveries++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"synthetic-model"},{"id":"synthetic-hidden","hidden":true}]}`))}, nil
	}))
	row, err := s.accounts.Authorize(ctx, "Synthetic Grok", accounts.Credential{Provider: "xai", AccountID: "synthetic-subject", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	id := row.ID
	if _, err := groups.New(s.db).Save(ctx, 1, groups.Input{Name: "Synthetic Grok", Enabled: true, AccountIDs: []string{id}}); err != nil {
		t.Fatal(err)
	}
	if err := s.accounts.SaveCatalog(ctx, id, 0, []string{"synthetic-model"}, s.now().Unix(), "xai:v1"); err != nil {
		t.Fatal(err)
	}
	models, err := s.Models(ctx, 1, 1)
	if err != nil || len(models) != 1 || models[0].ID != "synthetic-model" {
		t.Fatal("hidden model entered the public list", models, err)
	}
	restarted := New(ctx, s.db, s.accounts, s.provider)
	defer restarted.Close()
	for _, service := range []*Service{s, restarted} {
		selected, _, err := service.selectAccount(ctx, 1, 1, "", "xai", "synthetic-hidden", Responses)
		if err != nil || selected != id {
			t.Fatal("hidden capability did not survive discovery or restart", selected, err)
		}
		catalog, err := service.AccountCatalog(ctx, id, false)
		if err != nil || len(catalog.Models) != 1 || len(catalog.HiddenModels) != 0 {
			t.Fatal("account list exposed hidden capability metadata", catalog, err)
		}
		value, err := service.Availability(ctx, 1, 1, "synthetic-hidden")
		if err != nil || value.State != "available" {
			t.Fatal("hidden capability is unavailable to explicit diagnostics", value, err)
		}
	}
	if discoveries != 1 {
		t.Fatal("refreshed capabilities were not persisted", discoveries)
	}
	if _, _, err := restarted.selectAccount(ctx, 1, 1, "", "xai", "synthetic-unknown", Responses); !errors.Is(err, ErrModelUnavailable) {
		t.Fatal("unknown capability was admitted", err)
	}
	if err := restarted.loadRuntime(ctx); err != nil {
		t.Fatal(err)
	}
	restarted.health[id].InFlight = restarted.health[id].MaxConcurrency
	if _, _, err := restarted.selectAccount(ctx, 1, 1, "", "xai", "synthetic-hidden", Responses); !errors.Is(err, ErrAccountBusy) {
		t.Fatal("hidden model bypassed capacity", err)
	}
	restarted.health[id].InFlight = 0
	catalog, err := s.accounts.Catalog(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	key := modelKey{id, "synthetic-hidden"}
	restarted.modelHealth[key] = &modelRuntime{key: key, lifecycle: catalog.Revision, runtimeRevision: restarted.health[id].revision, failures: 1, cooldownUntil: s.now().Add(time.Minute).Unix(), persistFailed: true}
	if _, _, err := restarted.selectAccount(ctx, 1, 1, "", "xai", "synthetic-hidden", Responses); err == nil {
		t.Fatal("hidden model bypassed its cooldown")
	}
	if _, _, err := restarted.selectAccount(ctx, 1, 1, "", "xai", "synthetic-model", Responses); err != nil {
		t.Fatal("hidden model cooldown blocked an unrelated model", err)
	}
}
