package allocations

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/pricing"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
)

// This opt-in experiment calls the production ledger, not a reimplementation of
// its distribution algorithm. The independent oracle records synthetic upstream
// consumption before snapshots lose information through rounding or delay.
func TestAllocationSimulation(t *testing.T) {
	dir := os.Getenv("SUBLANE_SIMULATION_DIR")
	if dir == "" {
		t.Skip("set SUBLANE_SIMULATION_DIR to export the reproducible experiment")
	}
	scenarios := []simulationCase{
		{Name: "calibrated"},
		{Name: "model-mismatch", Mismatch: true},
		{Name: "rounded", Quantum: 100},
		{Name: "mismatch-rounded", Mismatch: true, Quantum: 100},
		{Name: "delayed", Mismatch: true, Quantum: 100, Lag: 45},
		{Name: "staggered-resets", Mismatch: true, Quantum: 100, Resets: true},
		{Name: "missing-usage", Missing: true},
		{Name: "external-usage", External: true},
		{Name: "weekly-heterogeneous", Weekly: true},
		{Name: "weekly-mismatch", Weekly: true, Mismatch: true},
		{Name: "weekly-rounded", Weekly: true, Quantum: 100},
		{Name: "weekly-rounded-borrow", Weekly: true, Quantum: 100, Borrow: true},
		{Name: "weekly-heavy", Weekly: true, Scale: 100},
		{Name: "weekly-heavy-rounded", Weekly: true, Scale: 100, Quantum: 100},
		{Name: "weekly-partial", Weekly: true, Partial: true},
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range scenarios {
		for _, seed := range []int64{101, 202, 303, 404, 505, 606, 707, 808} {
			t.Run(fmt.Sprintf("%s/%d", scenario.Name, seed), func(t *testing.T) {
				runAllocationSimulation(t, dir, scenario, seed)
			})
		}
	}
}

type simulationCase struct {
	Name                                string
	Mismatch, Resets, Missing, External bool
	Quantum                             float64 // Basis points; 100 means a displayed whole percentage.
	Lag                                 int64
	Weekly, Partial, Borrow             bool
	Scale                               int64
}
type simulationAccount struct {
	ID, Label     string
	Team          int
	Models        []string
	Capacity      [2]float64
	Period, Reset [2]int64
	WeeklyTokens  int64
	InitialUsed   [2]float64
}
type simulationEvent struct {
	Request  string
	Account  string
	User     int64
	Finished int64
	Reset    [2]int64
	Truth    [2]float64
}
type simulationAttempt struct {
	ID, Account, Model, Error string
	User                      int64
	Input, Output, Cached     int64
	Known                     bool
}
type simulationDebit struct {
	Request, Account, Kind, State string
	User, Reset, Allowance        int64
	Truth, Charged                float64
	Reconciled                    bool
}
type simulationOutput struct {
	Scenario          simulationCase
	Seed              int64
	Models            []Rate
	Factors           []float64
	Accounts          []simulationAccount
	Schemes           []Scheme
	Attempts          []simulationAttempt
	Events            []simulationEvent
	Debits            []simulationDebit
	Unassigned        int64
	ConservationError float64
	InitialResets     map[string][2]int64
}

func runAllocationSimulation(t *testing.T, dir string, scenario simulationCase, seed int64) {
	t.Helper()
	ctx := context.Background()
	rng := rand.New(rand.NewSource(seed))
	s, conn, firstUser, firstAccount := fixture(t)
	now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC).UnixMilli()
	s.now = func() time.Time { return time.UnixMilli(now) }
	q := db.New(conn)
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	result := simulationOutput{Scenario: scenario, Seed: seed,
		Models: []Rate{
			{Model: "mock-codex-mini", Input: 1250000, Cached: 125000, Output: 10000000},
			{Model: "mock-codex-standard", Input: 1750000, Cached: 175000, Output: 14000000},
			{Model: "mock-codex-reasoning", Input: 3000000, Cached: 300000, Output: 18000000},
			{Model: "mock-codex-pro", Input: 5000000, Cached: 500000, Output: 30000000},
			{Model: "mock-codex-max", Input: 10000000, Cached: 1000000, Output: 60000000}},
		Factors: []float64{1, 1, 1, 1, 1}}
	if scenario.Mismatch {
		result.Factors = []float64{0.35, 0.7, 1, 1.8, 3}
	}
	catalog := map[string]pricing.Price{}
	for _, m := range result.Models {
		catalog[m.Model] = pricing.Price{Input: m.Input, Cached: m.Cached, Output: m.Output}
	}
	s.pricing = pricing.NewStatic(catalog)
	users := []int64{firstUser}
	for i := 1; i < 9; i++ {
		res, err := conn.Exec("INSERT INTO users(username,role,password_hash,enabled,created_at) VALUES(?,'member','synthetic-not-a-login',1,1)", fmt.Sprintf("mock-member-%02d", i))
		check(err)
		id, err := res.LastInsertId()
		check(err)
		users = append(users, id)
	}
	rng.Shuffle(len(users), func(i, j int) { users[i], users[j] = users[j], users[i] })
	cipher, err := vault.Open(filepath.Join(t.TempDir(), "synthetic-key"), true)
	check(err)
	as := accounts.New(conn, cipher)
	for i := 0; i < 9; i++ {
		id := firstAccount
		if i > 0 {
			a, err := as.Authorize(ctx, fmt.Sprintf("Mock subscription %02d", i), accounts.Credential{AccountID: fmt.Sprintf("mock-sub-%02d", i), AccessToken: "mock-access", RefreshToken: "mock-refresh", ExpiresAt: 2000000000}, "")
			check(err)
			id = a.ID
		}
		order := rng.Perm(len(result.Models))
		models := []string{}
		for _, n := range order[:2+rng.Intn(4)] {
			models = append(models, result.Models[n].Model)
		}
		check(as.SaveCatalog(ctx, id, 0, models, now/1000, upstreamSimulationCatalog))
		shortPeriod := [2]int64{180 + rng.Int63n(180), 720 + rng.Int63n(360)}
		shortPhase := [2]int64{30 + rng.Int63n(shortPeriod[0]-30), 60 + rng.Int63n(shortPeriod[1]-60)}
		normalPhase := [2]int64{10000 + rng.Int63n(8000), 500000 + rng.Int63n(100000)}
		period, phase := [2]int64{18000, 604800}, normalPhase
		if scenario.Resets {
			period, phase = shortPeriod, shortPhase
		}
		cap := 80 + rng.Float64()*120
		result.Accounts = append(result.Accounts, simulationAccount{ID: id, Label: fmt.Sprintf("mock-sub-%02d", i), Models: models, Capacity: [2]float64{cap, cap * (3 + rng.Float64()*3)}, Period: period, Reset: [2]int64{now/1000 + phase[0], now/1000 + phase[1]}})
	}
	accountOrder := rng.Perm(9)
	_, err = groups.New(conn).Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{}})
	check(err)
	for team := 0; team < 3; team++ {
		ids := []string{}
		for position, index := range accountOrder[team*3 : (team+1)*3] {
			a := &result.Accounts[index]
			if scenario.Weekly {
				a.WeeklyTokens = []int64{1_000_000_000, 10_000_000_000, 3_000_000_000}[position]
				a.Capacity[1] = float64(a.WeeklyTokens) * 1.25 / 1e6
				a.Period[1], a.Reset[1] = 604800, now/1000+604800
			}
			if scenario.Partial {
				a.InitialUsed[1] = []float64{1500, 5000, 7500}[position]
			}
			result.Accounts[index].Team = team
			ids = append(ids, result.Accounts[index].ID)
		}
		poolID := int64(0)
		if team == 0 {
			poolID = 2
		}
		pool, err := groups.New(conn).Save(ctx, poolID, groups.Input{Name: fmt.Sprintf("Mock pool %d", team+1), Enabled: true, AccountIDs: ids})
		check(err)
		members := users[team*3 : (team+1)*3]
		personnel, err := s.SaveTeam(ctx, 0, TeamInput{Name: fmt.Sprintf("Mock team %d", team+1), Enabled: true, MemberIDs: members})
		check(err)
		weights := []int64{int64(10 + rng.Intn(50)), int64(10 + rng.Intn(50)), int64(10 + rng.Intn(50))}
		total := weights[0] + weights[1] + weights[2]
		remaining := int64(10000)
		shares := []Share{}
		for j, u := range members {
			share := 10000 * weights[j] / total
			if j == 2 {
				share = remaining
			}
			remaining -= share
			shares = append(shares, Share{UserID: u, Limit: share})
		}
		scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: fmt.Sprintf("Mock allocation %d", team+1), TeamID: personnel.ID, GroupID: pool.ID, Enabled: true, Config: Config{Mode: "ratio", Period: "upstream", AllowIdleBorrow: scenario.Borrow, Members: shares}})
		check(err)
		result.Schemes = append(result.Schemes, scheme)
	}
	// Refresh every account from the oracle; real Sync owns window creation,
	// timing checks, debt retention, largest remainders and reset behavior.
	observe := func() {
		for i := range result.Accounts {
			a := &result.Accounts[i]
			windows := []map[string]any{}
			for k, kind := range []string{"primary", "secondary"} {
				if scenario.Weekly && k == 0 {
					continue
				}
				for a.Reset[k] <= now/1000 {
					a.Reset[k] += a.Period[k]
				}
				used := 0.0
				if a.Reset[k] == result.InitialResets[a.ID][k] {
					used = a.InitialUsed[k]
				}
				for _, e := range result.Events {
					if e.Account == a.ID && e.Reset[k] == a.Reset[k] && e.Finished <= now-scenario.Lag*1000 {
						used += e.Truth[k]
					}
				}
				if scenario.Quantum > 0 {
					used = math.Floor((used+1e-9)/scenario.Quantum) * scenario.Quantum
				}
				windows = append(windows, map[string]any{"kind": kind, "used_percent": used / 100, "reset_at": a.Reset[k]})
			}
			raw, err := json.Marshal(map[string]any{"updated_at": now / 1000, "read_started_at": now, "limits": []any{map[string]any{"name": "", "windows": windows}}})
			check(err)
			row, err := q.GetAccountUsage(ctx, a.ID)
			check(err)
			_, err = q.SaveAccountUsage(ctx, db.SaveAccountUsageParams{AccountID: a.ID, Snapshot: raw, UpdatedAt: now / 1000, Revision: row.Revision})
			check(err)
		}
		for _, scheme := range result.Schemes {
			check(s.Refresh(ctx, scheme.ID))
		}
	}
	result.InitialResets = map[string][2]int64{}
	for _, a := range result.Accounts {
		result.InitialResets[a.ID] = a.Reset
	}
	observe()
	for round := 0; round < 20; round++ {
		for _, ai := range accountOrder {
			a := &result.Accounts[ai]
			scheme := result.Schemes[a.Team]
			burst := 3 + rng.Intn(5)
			for n := 0; n < burst; n++ {
				member := scheme.Config.Members[rng.Intn(3)]
				model := a.Models[rng.Intn(len(a.Models))]
				input := int64(1000 + rng.Intn(249001))
				output := int64(256 + rng.Intn(19745))
				cached := int64(float64(input) * rng.Float64() * 0.9)
				if scenario.Scale > 0 {
					input *= scenario.Scale
					output *= scenario.Scale
					cached *= scenario.Scale
				}
				missingDraw := rng.Float64()
				externalDraw := rng.Float64()
				id := fmt.Sprintf("mock-%d", len(result.Attempts)+1)
				attempt := simulationAttempt{ID: id, Account: a.ID, Model: model, User: member.UserID, Input: input, Output: output, Cached: cached, Known: !(scenario.Missing && missingDraw < 0.02)}
				err := Begin(ctx, q, Request{ID: id, SchemeID: scheme.ID, UserID: member.UserID, GroupID: scheme.GroupID, AccountID: a.ID, Model: model, StartedAt: now / 1000}, now/1000)
				if err != nil {
					attempt.Error = err.Error()
					result.Attempts = append(result.Attempts, attempt)
					continue
				}
				// The fake provider also enforces its real remaining capacity.
				// A stale local estimate must not create impossible unlimited upstream usage.
				exhausted := false
				for k := range 2 {
					if scenario.Weekly && k == 0 {
						continue
					}
					used := 0.0
					if a.Reset[k] == result.InitialResets[a.ID][k] {
						used = a.InitialUsed[k]
					}
					for _, event := range result.Events {
						if event.Account == a.ID && event.Reset[k] == a.Reset[k] {
							used += event.Truth[k]
						}
					}
					exhausted = exhausted || used >= 10000
				}
				if exhausted {
					check(Finish(ctx, q, id, Completion{Dispatched: true, Rejected: true}, now+50, false))
					attempt.Error = "mock_upstream_exhausted"
					result.Attempts = append(result.Attempts, attempt)
					result.Events = append(result.Events, simulationEvent{Request: id, Account: a.ID, User: member.UserID, Finished: now + 50, Reset: a.Reset})
					continue
				}
				e := simulationEvent{Request: id, Account: a.ID, User: member.UserID, Finished: now + 50, Reset: a.Reset}
				for mi, m := range result.Models {
					if m.Model == model {
						// Independent floating-point oracle. Do not call Cost/Distribute
						// here: differences in integer rounding must remain measurable.
						cost := (float64(input-cached)*float64(m.Input) + float64(cached)*float64(m.Cached) + float64(output)*float64(m.Output)) / 1e12 * result.Factors[mi]
						for k := range 2 {
							if !scenario.Weekly || k == 1 {
								e.Truth[k] = cost / a.Capacity[k] * 10000
							}
						}
					}
				}
				result.Events = append(result.Events, e)
				if scenario.External && externalDraw < 0.05 {
					extra := e
					extra.Request = "external-" + id
					extra.User = 0
					for k := range 2 {
						extra.Truth[k] *= 1.5
					}
					result.Events = append(result.Events, extra)
				}
				completion := Completion{Input: input, Output: output, Cached: cached, Known: attempt.Known, Dispatched: true}
				if !attempt.Known {
					completion.Input, completion.Output, completion.Cached = 0, 0, 0
				}
				check(Finish(ctx, q, id, completion, now+50, false))
				result.Attempts = append(result.Attempts, attempt)
				now += 100
			}
			now += 6000
			observe()
		}
	}
	// Let delayed observations catch up without fabricating a final exact reading
	// or manually settling debt. Rounded and expired debt remains in the report.
	now += (scenario.Lag + 6) * 1000
	observe()
	events := map[string]simulationEvent{}
	for _, e := range result.Events {
		events[e.Request] = e
	}
	rows, err := conn.Query(`SELECT e.request_id,e.account_id,e.user_id,e.state,w.kind,w.reset_at,d.points,d.reconciled,m.allowance FROM allocation_debits d JOIN allocation_entries e ON e.request_id=d.request_id JOIN allocation_windows w ON w.id=d.window_id JOIN allocation_window_members m ON m.window_id=w.id AND m.user_id=e.user_id ORDER BY e.request_id,w.kind`)
	check(err)
	for rows.Next() {
		var d simulationDebit
		var points, reconciled int64
		check(rows.Scan(&d.Request, &d.Account, &d.User, &d.State, &d.Kind, &d.Reset, &points, &reconciled, &d.Allowance))
		k := 0
		if d.Kind == "secondary" {
			k = 1
		}
		e := events[d.Request]
		if e.Reset[k] != d.Reset {
			t.Fatal("oracle window does not match actual ledger", d.Request)
		}
		d.Truth = e.Truth[k]
		d.Charged = float64(points)
		d.Reconciled = reconciled == 1
		result.Debits = append(result.Debits, d)
	}
	check(rows.Err())
	check(rows.Close())
	// Conservation is checked per window, never by inventing a combined capacity.
	rows, err = conn.Query(`SELECT w.id,w.observed_points-w.baseline_points,w.unassigned,COALESCE(sum(d.points),0) FROM allocation_windows w LEFT JOIN allocation_debits d ON d.window_id=w.id GROUP BY w.id`)
	check(err)
	for rows.Next() {
		var id, delta, unassigned, charged int64
		check(rows.Scan(&id, &delta, &unassigned, &charged))
		result.Unassigned += unassigned
		result.ConservationError += math.Abs(float64(delta - unassigned - charged))
	}
	check(rows.Err())
	check(rows.Close())
	if result.ConservationError != 0 {
		t.Fatalf("lost observed capacity: %g", result.ConservationError)
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	check(err)
	check(os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%d.json", scenario.Name, seed)), raw, 0600))
	t.Logf("seed=%d scenario=%s attempts=%d debits=%d conservation_error=%g", seed, scenario.Name, len(result.Attempts), len(result.Debits), result.ConservationError)
}

const upstreamSimulationCatalog = "synthetic simulation"

// These boundary probes document existing behavior without changing the allocator.
func TestAllocationSimulationBoundaries(t *testing.T) {
	for _, name := range []string{"short-window-exhausted", "completion-crosses-reset", "membership-and-share-change"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s, conn, user, account := fixture(t)
			q := db.New(conn)
			now := int64(1_900_000_000)
			s.now = func() time.Time { return unix(now) }
			check := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic boundary", Enabled: true, MemberIDs: []int64{user}})
			check(err)
			config := Config{Mode: "ratio", Period: "upstream", Members: []Share{{UserID: user, Limit: 10000}}, Rates: []Rate{{Model: "synthetic", Input: 1_000_000, Cached: 100_000, Output: 8_000_000}}}
			scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic boundary", TeamID: team.ID, GroupID: 2, Enabled: true, Config: config})
			check(err)
			resets := [2]int64{now + 60, now + 604800}
			snapshot := func(primary, secondary float64) {
				t.Helper()
				raw, err := json.Marshal(map[string]any{"updated_at": now, "read_started_at": now * 1000, "limits": []any{map[string]any{"name": "", "windows": []any{map[string]any{"kind": "primary", "used_percent": primary, "reset_at": resets[0]}, map[string]any{"kind": "secondary", "used_percent": secondary, "reset_at": resets[1]}}}}})
				check(err)
				row, err := q.GetAccountUsage(ctx, account)
				check(err)
				_, err = q.SaveAccountUsage(ctx, db.SaveAccountUsageParams{AccountID: account, Snapshot: raw, UpdatedAt: now, Revision: row.Revision})
				check(err)
				check(s.Refresh(ctx, scheme.ID))
			}
			if name == "short-window-exhausted" {
				snapshot(100, 5)
				_, err := Check(ctx, q, scheme.ID, user, 2, account, "synthetic", now)
				if err != ErrQuota {
					t.Fatalf("short window must block despite 95%% weekly remaining: %v", err)
				}
				t.Log("short window exhausted + 95% weekly remaining => allocation_exhausted")
				return
			}
			snapshot(0, 0)
			if name == "completion-crosses-reset" {
				oldReset := resets[0]
				check(Begin(ctx, q, Request{ID: "synthetic-crossing", SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic", StartedAt: now}, now))
				now = oldReset + 1
				resets[0] += 18000
				check(Finish(ctx, q, "synthetic-crossing", Completion{Input: 100000, Known: true, Dispatched: true}, now*1000, false))
				now++
				snapshot(1, 1)
				entry, err := q.GetAllocationEntry(ctx, "synthetic-crossing")
				check(err)
				debits, err := q.GetRequestAllocationDebits(ctx, "synthetic-crossing")
				check(err)
				if entry.State != "pending" {
					t.Fatalf("cross-window debt state: %s", entry.State)
				}
				for _, d := range debits {
					if d.Kind == "primary" && (d.ResetAt != oldReset || d.Reconciled != 0) {
						t.Fatal("crossing request silently moved/settled", d)
					}
				}
				_, err = Check(ctx, q, scheme.ID, user, 2, account, "synthetic", now)
				if err != ErrPending {
					t.Fatalf("unresolved crossing admitted: %v", err)
				}
				t.Log("completion after reset => old-window debt retained; requires settlement; next admission pending")
				return
			}
			res, err := conn.Exec("INSERT INTO users(username,role,password_hash,enabled,created_at) VALUES('synthetic-new-member','member','synthetic',1,1)")
			check(err)
			newcomer, err := res.LastInsertId()
			check(err)
			_, err = s.SaveTeam(ctx, team.ID, TeamInput{Name: "Synthetic boundary", Enabled: true, MemberIDs: []int64{user, newcomer}})
			check(err)
			config.Members = []Share{{UserID: user, Limit: 5000}, {UserID: newcomer, Limit: 5000}}
			_, err = s.SaveScheme(ctx, scheme.ID, SchemeInput{Name: "Synthetic boundary", TeamID: team.ID, GroupID: 2, Enabled: true, Config: config})
			check(err)
			rev, err := Current(ctx, q, scheme.ID, now)
			check(err)
			if len(rev.Config.Members) != 1 || rev.Config.Members[0].Limit != 10000 {
				t.Fatal("edit rewrote active allowance")
			}
			_, err = Check(ctx, q, scheme.ID, newcomer, 2, account, "synthetic", now)
			if err != ErrUnavailable {
				t.Fatalf("new member admitted before revision: %v", err)
			}
			now = resets[0] + 1
			resets[0] += 18000
			snapshot(0, 0)
			rev, err = Current(ctx, q, scheme.ID, now)
			check(err)
			if len(rev.Config.Members) != 1 {
				t.Fatal("short reset activated weekly revision early")
			}
			now = resets[1] + 1
			resets = [2]int64{now + 18000, now + 604800}
			snapshot(0, 0)
			_, err = Check(ctx, q, scheme.ID, newcomer, 2, account, "synthetic", now)
			check(err)
			detail, err := s.Detail(ctx, scheme.ID, 0)
			check(err)
			for _, b := range detail.Balances {
				if b.Limit != 5000 {
					t.Fatal("wrong new share", b)
				}
			}
			t.Log("new member + 100% to 50/50 edit => deferred until latest old window reset; then admitted at 50%")
		})
	}
}
