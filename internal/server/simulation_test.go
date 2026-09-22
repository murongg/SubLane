package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/upstream"
)

type simulationHTTPCall struct {
	User                    int64
	Model, Account, Session string
	Input, Output, Cached   int64
	Truth                   [2]float64
	Status                  int
}
type simulationHTTPResult struct {
	Seed           int64
	Mismatch       bool
	Rates          []allocations.Rate
	Factors        []float64
	Calls          []simulationHTTPCall
	Details        []allocations.Detail
	ElapsedSeconds float64
}

// A separate real-time HTTP cross-check keeps the fast ledger experiment honest:
// requests traverse bearer authentication, routing, SDK execution, SSE parsing,
// durable completion and production quota refresh. Only the upstream is fake.
func TestAllocationHTTPSimulation(t *testing.T) {
	dir := os.Getenv("SUBLANE_SIMULATION_DIR")
	if dir == "" {
		t.Skip("set SUBLANE_SIMULATION_DIR to run the HTTP experiment")
	}
	for _, mismatch := range []bool{false, true} {
		for _, seed := range []int64{101, 202, 303} {
			t.Run(fmt.Sprintf("mismatch-%v/%d", mismatch, seed), func(t *testing.T) {
				t.Parallel()
				runHTTPSimulation(t, dir, seed, mismatch)
			})
		}
	}
}

func runHTTPSimulation(t *testing.T, dir string, seed int64, mismatch bool) {
	started := time.Now()
	ctx := context.Background()
	rng := rand.New(rand.NewSource(seed))
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	result := simulationHTTPResult{Seed: seed, Mismatch: mismatch,
		Rates: []allocations.Rate{
			{Model: "mock-codex-mini", Input: 1250000, Cached: 125000, Output: 10000000},
			{Model: "mock-codex-standard", Input: 1750000, Cached: 175000, Output: 14000000},
			{Model: "mock-codex-reasoning", Input: 3000000, Cached: 300000, Output: 18000000},
			{Model: "mock-codex-pro", Input: 5000000, Cached: 500000, Output: 30000000},
			{Model: "mock-codex-max", Input: 10000000, Cached: 1000000, Output: 60000000}},
		Factors: []float64{1, 1, 1, 1, 1}}
	if mismatch {
		result.Factors = []float64{0.35, 0.7, 1, 1.8, 3}
	}
	var mu sync.Mutex
	current := simulationHTTPCall{}
	used := map[string][2]float64{}
	identities := map[string]string{}
	reset := time.Now().Unix() + 18000
	f := newQuotaForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		subject := r.Header.Get("Chatgpt-Account-Id")
		if r.URL.Path == "/backend-api/wham/usage" {
			v := used[subject]
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"rate_limit": map[string]any{
				"primary_window":   map[string]any{"used_percent": v[0] / 100, "limit_window_seconds": 18000, "reset_at": reset},
				"secondary_window": map[string]any{"used_percent": v[1] / 100, "limit_window_seconds": 604800, "reset_at": reset + 586800}}})
			return
		}
		if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/responses") {
			http.Error(w, "unexpected mock route", 500)
			return
		}
		var body struct {
			Model string `json:"model"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != current.Model || identities[subject] == "" {
			http.Error(w, "mock request mismatch", 500)
			return
		}
		call := current
		call.Account = identities[subject]
		for i, rate := range result.Rates {
			if rate.Model == call.Model {
				cost := (float64(call.Input-call.Cached)*float64(rate.Input) + float64(call.Cached)*float64(rate.Cached) + float64(call.Output)*float64(rate.Output)) / 1e12 * result.Factors[i]
				call.Truth = [2]float64{cost / 200 * 10000, cost / 1000 * 10000}
			}
		}
		v := used[subject]
		for k := range 2 {
			v[k] += call.Truth[k]
		}
		used[subject] = v
		current = call
		w.Header().Set("Content-Type", "text/event-stream")
		frame, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"id": "mock-response", "output": []any{}, "usage": map[string]any{"input_tokens": call.Input, "output_tokens": call.Output, "input_tokens_details": map[string]any{"cached_tokens": call.Cached}}}})
		fmt.Fprintf(w, "data: %s\n\n", frame)
	})
	users := []int64{f.userID}
	for i := 1; i < 9; i++ {
		u, err := f.identity.CreateMember(ctx, fmt.Sprintf("mock-http-member-%d", i), "synthetic-pass")
		check(err)
		users = append(users, u.ID)
	}
	rng.Shuffle(len(users), func(i, j int) { users[i], users[j] = users[j], users[i] })
	all, err := f.accounts.List(ctx)
	check(err)
	ids := []string{all[0].ID}
	identities["synthetic-upstream-account"] = all[0].ID
	for i := 1; i < 6; i++ {
		subject := fmt.Sprintf("mock-http-sub-%d", i)
		a, err := f.accounts.Authorize(ctx, subject, accounts.Credential{AccountID: subject, AccessToken: "mock-access", RefreshToken: "mock-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
		check(err)
		ids = append(ids, a.ID)
		identities[subject] = a.ID
	}
	models := []string{}
	for _, rate := range result.Rates {
		models = append(models, rate.Model)
	}
	for _, id := range ids {
		check(f.accounts.SaveCatalog(ctx, id, 0, models, time.Now().Unix(), upstream.CatalogSource("codex")))
	}
	rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	_, err = f.groups.Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{}})
	check(err)
	manager := f.forwarding.Allocations()
	schemes := []int64{}
	secrets := map[int64]string{}
	for team := 0; team < 3; team++ {
		pool, err := f.groups.Save(ctx, 0, groups.Input{Name: fmt.Sprintf("Mock HTTP pool %d", team), Enabled: true, AccountIDs: ids[team*2 : (team+1)*2]})
		check(err)
		members := users[team*3 : (team+1)*3]
		personnel, err := manager.SaveTeam(ctx, 0, allocations.TeamInput{Name: fmt.Sprintf("Mock HTTP team %d", team), Enabled: true, MemberIDs: members})
		check(err)
		first := int64(2000 + rng.Intn(2500))
		second := int64(1500 + rng.Intn(2500))
		shares := []allocations.Share{{UserID: members[0], Limit: first}, {UserID: members[1], Limit: second}, {UserID: members[2], Limit: 10000 - first - second}}
		scheme, err := manager.SaveScheme(ctx, 0, allocations.SchemeInput{Name: fmt.Sprintf("Mock HTTP allocation %d", team), TeamID: personnel.ID, GroupID: pool.ID, Enabled: true, Config: allocations.Config{Mode: "ratio", Period: "upstream", Members: shares, Rates: result.Rates}})
		check(err)
		schemes = append(schemes, scheme.ID)
		check(f.forwarding.RefreshAllocation(ctx, scheme.ID))
		for _, user := range members {
			key, err := f.keys.CreateInScheme(ctx, user, scheme.ID, "Mock HTTP key", nil)
			check(err)
			secrets[user] = key.Secret
		}
	}
	// Keep each batch below the existing per-account admission bound. Multiple
	// members reuse sticky sessions while models and token/cache mixes vary.
	for batch := 0; batch < 4; batch++ {
		for _, user := range users {
			for turn := 0; turn < 2; turn++ {
				input := int64(1000 + rng.Intn(249001))
				out := int64(256 + rng.Intn(19745))
				cached := int64(float64(input) * rng.Float64() * 0.9)
				call := simulationHTTPCall{User: user, Model: models[rng.Intn(len(models))], Input: input, Output: out, Cached: cached, Session: fmt.Sprintf("mock-session-%d", user)}
				mu.Lock()
				current = call
				mu.Unlock()
				raw, _ := json.Marshal(map[string]any{"model": call.Model, "input": []any{}, "stream": true})
				req, err := http.NewRequest("POST", f.server.URL+"/v1/responses", strings.NewReader(string(raw)))
				check(err)
				req.Header.Set("Authorization", "Bearer "+secrets[user])
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Session_id", call.Session)
				response, err := http.DefaultClient.Do(req)
				check(err)
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				check(err)
				mu.Lock()
				call = current
				call.Status = response.StatusCode
				result.Calls = append(result.Calls, call)
				mu.Unlock()
				if response.StatusCode != 200 || !strings.Contains(string(body), "response.completed") {
					t.Fatalf("HTTP simulation rejected: %d %s", response.StatusCode, body)
				}
			}
		}
		// Real timers exercise the production cooldown and automatic sync worker.
		time.Sleep(6 * time.Second)
		for _, id := range schemes {
			check(f.forwarding.RefreshAllocation(ctx, id))
		}
	}
	bindings := map[int64]string{}
	for _, call := range result.Calls {
		if prior := bindings[call.User]; prior != "" && prior != call.Account {
			t.Fatal("sticky session moved accounts")
		}
		bindings[call.User] = call.Account
	}
	for _, id := range schemes {
		d, err := manager.Detail(ctx, id, 0)
		check(err)
		if len(d.Pending) > 0 || d.Unassigned > 0 {
			t.Fatal("HTTP experiment did not finish settling", d.Pending, d.Unassigned)
		}
		result.Details = append(result.Details, d)
	}
	result.ElapsedSeconds = time.Since(started).Seconds()
	raw, err := json.MarshalIndent(result, "", "  ")
	check(err)
	check(os.MkdirAll(dir, 0700))
	check(os.WriteFile(filepath.Join(dir, fmt.Sprintf("http-%v-%d.json", mismatch, seed)), raw, 0600))
	t.Logf("seed=%d mismatch=%v successful_http_requests=%d elapsed=%.1fs", seed, mismatch, len(result.Calls), result.ElapsedSeconds)
}
