package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
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
		if err != nil || len(catalog.Models) != 1 || (len(catalog.HiddenModels) != 1 || catalog.HiddenModels[0] != "synthetic-internal") {
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
}

func TestHiddenGeminiQuotaRefreshControlsOpen(t *testing.T) {
	for _, test := range []struct {
		name              string
		internal, blocked bool
		remaining         *float64
		resetPast         bool
	}{
		{"visible exhausted", false, true, quotaFloat(0), false},
		{"internal exhausted", true, true, quotaFloat(0), false},
		{"internal available", true, false, quotaFloat(.75), false},
		{"internal unknown", true, false, nil, false},
		{"internal reset", true, false, quotaFloat(0), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			fixture, ids := discoveryGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("unexpected fixture IO")
			}))
			reset := time.Now().Add(90 * time.Second)
			if test.resetPast {
				reset = time.Now().Add(-time.Minute)
			}
			quota := map[string]any{}
			if test.remaining != nil {
				quota["remainingFraction"] = *test.remaining
				quota["resetTime"] = reset.UTC().Format(time.RFC3339)
			}
			body, err := json.Marshal(map[string]any{"models": map[string]any{
				"synthetic-internal": map[string]any{"isInternal": test.internal, "quotaInfo": quota},
				"synthetic-visible":  map[string]any{"quotaInfo": map[string]any{"remainingFraction": .75}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			var internalCalls, visibleCalls atomic.Int32
			client := upstream.NewWithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v1internal:fetchAvailableModels" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
				}
				var input struct{ Model string }
				if json.NewDecoder(r.Body).Decode(&input) != nil {
					return nil, errors.New("invalid synthetic inference")
				}
				switch input.Model {
				case "synthetic-internal":
					internalCalls.Add(1)
				case "synthetic-visible":
					visibleCalls.Add(1)
				default:
					return nil, errors.New("unexpected synthetic model")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"response\":{\"responseId\":\"synthetic-response\",\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"Synthetic response\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":1}}}\n\n"))}, nil
			}))
			defer client.Close()
			s := New(ctx, fixture.db, fixture.accounts, client)
			defer s.Close()
			id := ids["antigravity"]
			if _, err := groups.New(s.db).Save(ctx, 1, groups.Input{Name: "Synthetic Gemini", Enabled: true, AccountIDs: []string{id}}); err != nil {
				t.Fatal(err)
			}
			if catalog, err := s.AccountCatalog(ctx, id, true); err != nil || catalog.RefreshFailed {
				t.Fatal(catalog, err)
			}
			refreshed, err := s.RefreshUsage(ctx, id)
			if err != nil || refreshed.RefreshFailed || refreshed.Stale {
				t.Fatal(refreshed, err)
			}
			restarted := New(ctx, s.db, s.accounts, client)
			defer restarted.Close()
			for _, service := range []*Service{s, restarted} {
				stream, err := service.Open(ctx, 1, 1, []byte(`{"model":"synthetic-internal","input":"Synthetic operation"}`), nil, Responses)
				if stream != nil {
					stream.Body.Close()
				}
				if test.blocked && !errors.Is(err, ErrQuotaExhausted) || !test.blocked && err != nil {
					t.Fatalf("RefreshUsage -> Open: blocked=%t, got %v", test.blocked, err)
				}
				usage, err := service.Usage(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(usage)
				if err != nil || test.internal && (len(usage.Limits) != 1 || strings.Contains(string(encoded), "synthetic-internal")) || !test.internal && len(usage.Limits) != 2 {
					t.Fatal("internal quota entered the usage presentation", string(encoded), err)
				}
				row, err := service.queries.GetAccountUsage(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				stored := savedUsage(row)
				if stored == nil || len(stored.Limits) != 2 || test.internal && !strings.Contains(string(row.Snapshot), `"hidden":true`) {
					t.Fatal("persisted quota lost internal limits or visibility metadata")
				}
				service.usage.mu.Lock()
				cached := len(service.usage.entries[id].snapshot.Limits)
				service.usage.mu.Unlock()
				if cached != 2 {
					t.Fatal("presentation changed the shared quota cache", cached)
				}
				availability, err := service.Availability(ctx, 1, 1, "synthetic-internal")
				if err != nil || test.blocked && availability.State != "unavailable" || !test.blocked && availability.State != "available" {
					t.Fatal("availability disagrees with refreshed quota", availability, err)
				}
				stream, err = service.Open(ctx, 1, 1, []byte(`{"model":"synthetic-visible","input":"Synthetic operation"}`), nil, Responses)
				if err != nil {
					t.Fatal("internal quota blocked an unrelated model", err)
				}
				stream.Body.Close()
			}
			if test.blocked && internalCalls.Load() != 0 || !test.blocked && internalCalls.Load() != 2 || visibleCalls.Load() != 2 {
				t.Fatal("incorrect inference dispatch", internalCalls.Load(), visibleCalls.Load())
			}
		})
	}
}

func TestHiddenGeminiQuotaLargeSnapshotSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	fixture, ids := discoveryGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unexpected fixture IO") }))
	models := map[string]any{}
	for index := range 512 {
		id := fmt.Sprintf("synthetic-%03d-", index) + strings.Repeat("x", 114)
		models[id] = map[string]any{"isInternal": index != 0, "quotaInfo": map[string]any{"remainingFraction": .75}}
	}
	body, _ := json.Marshal(map[string]any{"models": models})
	var calls atomic.Int32
	client := upstream.NewWithTransport(transportFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	}))
	defer client.Close()
	s := New(ctx, fixture.db, fixture.accounts, client)
	defer s.Close()
	id := ids["antigravity"]
	first, err := s.RefreshUsage(ctx, id)
	if err != nil || first.RefreshFailed || len(first.Limits) != 1 {
		t.Fatal("large hidden quota snapshot failed", first, err)
	}
	row, err := s.queries.GetAccountUsage(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	stored := savedUsage(row)
	if len(row.Snapshot) <= 128<<10 || stored == nil || len(stored.Limits) != 512 {
		t.Fatal("large internal quota snapshot cannot be restored", len(row.Snapshot))
	}
	restarted := New(ctx, s.db, s.accounts, client)
	defer restarted.Close()
	second, err := restarted.Usage(ctx, id)
	if err != nil || len(second.Limits) != 1 || calls.Load() != 1 || second.UpdatedAt != first.UpdatedAt {
		t.Fatal("restart lost acknowledged internal quota data", second, calls.Load(), err)
	}
}

func TestHiddenGeminiQuotaPresentationPreservesCache(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	var usage upstream.Usage
	raw := fmt.Sprintf(`{"updated_at":%d,"limits":[{"name":"synthetic-internal","hidden":true,"windows":[{"kind":"model","used_percent":100,"reset_at":%d}]},{"name":"synthetic-visible","windows":[{"kind":"model","used_percent":25}]}]}`, now.Unix(), now.Add(time.Minute).Unix())
	if err := json.Unmarshal([]byte(raw), &usage); err != nil {
		t.Fatal(err)
	}
	entry := &usageEntry{snapshot: &usage}
	first := snapshotState(entry, now)
	if len(first.Limits) != 1 || first.Limits[0].Name != "synthetic-visible" || first.ExpiresAt != now.Add(time.Minute).Unix() {
		t.Fatal("hidden limits leaked or stopped contributing freshness", first)
	}
	first.Limits[0].Name = "synthetic-modified-view"
	second := snapshotState(entry, now)
	if len(usage.Limits) != 2 || len(second.Limits) != 1 || second.Limits[0].Name != "synthetic-visible" {
		t.Fatal("presentation mutated the complete cached limits", usage, second)
	}
	decision := quotaStatus(quotaForModel(usage, "antigravity", "synthetic-internal"), now)
	if decision.State != "exhausted" {
		t.Fatal("presentation cleared hidden quota admission", decision)
	}
}
