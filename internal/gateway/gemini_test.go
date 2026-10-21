package gateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/upstream"
)

func TestHiddenGeminiCapabilitiesPersistAndKeepAdmissionChecks(t *testing.T) {
	ctx := context.Background()
	discoveries := 0
	s, ids := discoveryGateway(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "daily-cloudcode-pa.googleapis.com" {
			t.Fatal("unexpected provider IO", r.URL.Host)
		}
		discoveries++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"models":{"synthetic-model":{},"synthetic-internal":{"isInternal":true}}}`))}, nil
	}))
	id := ids["antigravity"]
	if _, err := groups.New(s.db).Save(ctx, 1, groups.Input{Name: "Synthetic Gemini", Enabled: true, AccountIDs: []string{id}}); err != nil {
		t.Fatal(err)
	}
	if err := s.accounts.SaveCatalog(ctx, id, 0, []string{"synthetic-model", "synthetic-internal"}, s.now().Unix(), "antigravity:v1"); err != nil {
		t.Fatal(err)
	}
	models, err := s.Models(ctx, 1, 1)
	if err != nil || len(models) != 1 || models[0].ID != "synthetic-model" {
		t.Fatal("legacy catalog exposed an internal model", models, err)
	}
	restarted := New(ctx, s.db, s.accounts, s.provider)
	defer restarted.Close()
	for _, service := range []*Service{s, restarted} {
		selected, _, err := service.selectAccount(ctx, 1, 1, "", "antigravity", "synthetic-internal", Responses)
		if err != nil || selected != id {
			t.Fatal("internal capability did not survive discovery or restart", selected, err)
		}
		catalog, err := service.AccountCatalog(ctx, id, false)
		if err != nil || len(catalog.Models) != 1 || len(catalog.HiddenModels) != 0 {
			t.Fatal("account list exposed internal capability metadata", catalog, err)
		}
		value, err := service.Availability(ctx, 1, 1, "synthetic-internal")
		if err != nil || value.State != "available" {
			t.Fatal("explicit internal capability is unavailable", value, err)
		}
	}
	if discoveries != 1 {
		t.Fatal("refreshed capabilities were not persisted", discoveries)
	}
	if _, _, err := restarted.selectAccount(ctx, 1, 1, "", "antigravity", "synthetic-unknown", Responses); !errors.Is(err, ErrModelUnavailable) {
		t.Fatal("unknown capability was admitted", err)
	}
	if err := restarted.loadRuntime(ctx); err != nil {
		t.Fatal(err)
	}
	restarted.health[id].InFlight = restarted.health[id].MaxConcurrency
	if _, _, err := restarted.selectAccount(ctx, 1, 1, "", "antigravity", "synthetic-internal", Responses); !errors.Is(err, ErrAccountBusy) {
		t.Fatal("hidden capability bypassed capacity", err)
	}
	restarted.health[id].InFlight = 0
	saveProviderQuota(t, s, id, upstream.Usage{UpdatedAt: s.now().Unix(), Limits: []upstream.UsageLimit{{Name: "synthetic-internal", Windows: []upstream.UsageWindow{{UsedPercent: quotaFloat(100), ResetAt: quotaInt(s.now().Add(time.Hour).Unix())}}}}})
	limited := New(ctx, s.db, s.accounts, s.provider)
	defer limited.Close()
	if _, _, err := limited.selectAccount(ctx, 1, 1, "", "antigravity", "synthetic-internal", Responses); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatal("hidden capability bypassed its model quota", err)
	}
	if _, _, err := limited.selectAccount(ctx, 1, 1, "", "antigravity", "synthetic-model", Responses); err != nil {
		t.Fatal("hidden model quota blocked an unrelated model", err)
	}
}
