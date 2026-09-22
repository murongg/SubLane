package allocations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
)

func fixture(t *testing.T) (*Service, *sql.DB, int64, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	conn, err := storage.Open(ctx, filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	a, err := auth.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Setup(ctx, "synthetic-admin", "synthetic-pass"); err != nil {
		t.Fatal(err)
	}
	u, err := a.CreateMember(ctx, "synthetic-member", "synthetic-pass")
	if err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	account, err := accounts.New(conn, v).Authorize(ctx, "Synthetic", accounts.Credential{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", AccountID: "synthetic-subject", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := groups.New(conn).Save(ctx, 0, groups.Input{Name: "Synthetic pool", Enabled: true, AccountIDs: []string{account.ID}})
	if err != nil || pool.ID != 2 {
		t.Fatal(pool, err)
	}
	if _, err = groups.New(conn).Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	return New(conn), conn, u.ID, account.ID
}
func TestTeamsSchemesAndExclusiveCapacity(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic team", Enabled: true, MemberIDs: []int64{user}})
	if err != nil {
		t.Fatal(err)
	}
	input := SchemeInput{Name: "Synthetic scheme", TeamID: team.ID, GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "month", Members: []Share{{UserID: user, Limit: 1500000}}, Rates: []Rate{}}}
	scheme, err := s.SaveScheme(ctx, 0, input)
	if err != nil {
		t.Fatal(err)
	}
	if scheme.Config.Mode != "tokens" || scheme.Config.Members[0].Limit != 1500000 {
		t.Fatal(scheme)
	}
	if _, err = s.SaveScheme(ctx, 0, input); !errors.Is(err, ErrPoolConflict) {
		t.Fatal("duplicated pool budget", err)
	}
	if _, err = groups.New(conn).Save(ctx, 0, groups.Input{Name: "Conflicting", Enabled: true, AccountIDs: []string{account}}); err == nil {
		t.Fatal("shared account escaped exclusive scheme")
	}
	input.Config.Mode = "ratio"
	input.Config.Period = "upstream"
	input.Config.Members[0].Limit = 10001
	if _, err = s.SaveScheme(ctx, scheme.ID, input); !errors.Is(err, ErrInput) {
		t.Fatal("overallocated ratio", err)
	}
	input.Config.Mode = "amount"
	input.Config.Period = "month"
	input.Config.Members[0].Limit = 1000000
	if _, err = s.SaveScheme(ctx, scheme.ID, input); !errors.Is(err, ErrInput) {
		t.Fatal("unpriced amount scheme", err)
	}
}
func TestCostSeparatesCacheAndPreservesSmallCharges(t *testing.T) {
	r := Rate{Model: "synthetic-model", Input: 2000000, Cached: 200000, Output: 8000000}
	cost, err := Cost(r, 100, 50, 40)
	if err != nil || cost != 528 {
		t.Fatal(cost, err)
	}
	cost, err = Cost(Rate{Input: 1, Cached: 1, Output: 1}, 1, 0, 0)
	if err != nil || cost != 1 {
		t.Fatal("lost fractional charge", cost, err)
	}
	if _, err = Cost(r, 10, 0, 11); !errors.Is(err, ErrInput) {
		t.Fatal("invalid cached subset", err)
	}
}
func TestRatioDistributionConservesObservedPoints(t *testing.T) {
	got, err := Distribute(1200, []int64{400, 100, 100})
	if err != nil || got[0] != 800 || got[1] != 200 || got[2] != 200 {
		t.Fatal(got, err)
	}
	got, err = Distribute(1, []int64{1, 1, 1})
	if err != nil || got[0]+got[1]+got[2] != 1 {
		t.Fatal("rounding lost points", got, err)
	}
	if _, err = Distribute(100, []int64{0, 0}); err == nil {
		t.Fatal("unknown consumption treated as free")
	}
}
func TestSchemeRevisionDoesNotResetCurrentAllowance(t *testing.T) {
	s, conn, user, _ := fixture(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic team", Enabled: true, MemberIDs: []int64{user}})
	if err != nil {
		t.Fatal(err)
	}
	input := SchemeInput{Name: "Synthetic scheme", TeamID: team.ID, GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "month", Members: []Share{{UserID: user, Limit: 100}}}}
	scheme, err := s.SaveScheme(ctx, 0, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Config.Mode = "amount"
	input.Config.Members[0].Limit = 200
	input.Config.Rates = []Rate{{Model: "synthetic-model", Input: 1, Cached: 1, Output: 1}}
	updated, err := s.SaveScheme(ctx, scheme.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Config.Mode != "tokens" || updated.Next == nil || updated.Next.Config.Mode != "amount" {
		t.Fatal("mode changed current period", updated)
	}
	now = time.Date(2026, 9, 1, 0, 0, 1, 0, time.UTC)
	rev, err := Current(ctx, db.New(conn), scheme.ID, now.Unix())
	if err != nil || rev.Config.Mode != "amount" {
		t.Fatal(rev, err)
	}
}

func TestAccountingIsolationPendingRecoveryAndRollover(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	now := time.Now().Unix()
	q := db.New(conn)
	team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic team", Enabled: true, MemberIDs: []int64{user}})
	if err != nil {
		t.Fatal(err)
	}
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic budget", TeamID: team.ID, GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 5}}}})
	if err != nil {
		t.Fatal(err)
	}
	r := Request{ID: "synthetic-request", SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic-model", StartedAt: now}
	if err = Begin(ctx, q, r, now); err != nil {
		t.Fatal(err)
	}
	if err = Finish(ctx, q, r.ID, Completion{Input: 3, Output: 2, Known: true, Dispatched: true}, now*1000, false); err != nil {
		t.Fatal(err)
	}
	if _, err = Check(ctx, q, scheme.ID, user, 2, account, r.Model, now); !errors.Is(err, ErrQuota) {
		t.Fatal("exhausted allowance admitted", err)
	}
	if _, err = Check(ctx, q, scheme.ID, user, 2, account, r.Model, now+86400); err != nil {
		t.Fatal("daily reset", err)
	}
	r.ID = "synthetic-pending"
	if err = Begin(ctx, q, r, now+86400); err != nil {
		t.Fatal(err)
	}
	if err = q.RecoverAllocationEntries(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = Check(ctx, q, scheme.ID, user, 2, account, r.Model, now+172800); !errors.Is(err, ErrPending) {
		t.Fatal("restart lost unsettled debt", err)
	}
	if err = s.Settle(ctx, scheme.ID, r.ID, Completion{Input: 3, Output: 2}, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.Settle(ctx, scheme.ID, r.ID, Completion{Input: 3, Output: 2}, nil); err != nil {
		t.Fatal("settlement retry", err)
	}
	if _, err = Check(ctx, q, scheme.ID, user, 2, account, r.Model, now+172800); err != nil {
		t.Fatal("charged next period", err)
	}
}

func TestManagedPoolRejectsNewAccountsAndPausesWithoutQuotaSnapshot(t *testing.T) {
	s, conn, user, _ := fixture(t)
	ctx := context.Background()
	team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic", Enabled: true, MemberIDs: []int64{user}})
	if err != nil {
		t.Fatal(err)
	}
	in := SchemeInput{Name: "Synthetic", TeamID: team.ID, GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", Period: "upstream", Members: []Share{{UserID: user, Limit: 10000}}, Rates: []Rate{{Model: "synthetic", Input: 1, Cached: 1, Output: 1}}}}
	scheme, err := s.SaveScheme(ctx, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetEnabled(ctx, scheme.ID, false); err != nil {
		t.Fatal("cannot pause during quota outage", err)
	}
	if _, err = conn.Exec("INSERT INTO accounts(id,name,provider,account_id,credential,email,plan,enabled,status,created_at,updated_at,models_revision) VALUES('synthetic-new','Synthetic','claude','synthetic-new',x'00','','',1,'ready',1,1,0)"); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec("INSERT INTO group_accounts(group_id,account_id) VALUES(2,'synthetic-new')"); err == nil {
		t.Fatal("managed default pool silently gained account")
	}
}

func TestDefaultPoolCannotBeReserved(t *testing.T) {
	s, _, user, _ := fixture(t)
	ctx := context.Background()
	team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic", Enabled: true, MemberIDs: []int64{user}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic", TeamID: team.ID, GroupID: 1, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}})
	if !errors.Is(err, ErrPoolConflict) {
		t.Fatal("default pool must stay open for account imports", err)
	}
}

func TestTokenCorrectionRejectsOversizedUsage(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic", Enabled: true, MemberIDs: []int64{user}})
	if err != nil {
		t.Fatal(err)
	}
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic", TeamID: team.ID, GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(conn)
	now := time.Now().Unix()
	if err = Begin(ctx, q, Request{ID: "synthetic-large", SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic", StartedAt: now}, now); err != nil {
		t.Fatal(err)
	}
	if err = q.RecoverAllocationEntries(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Settle(ctx, scheme.ID, "synthetic-large", Completion{Input: 1_000_000_001}, nil); !errors.Is(err, ErrInput) {
		t.Fatal("oversized manual usage accepted", err)
	}
}

func TestRetentionKeepsUnresolvedDebtAndCurrentBalances(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	now := time.Now().Unix()
	s.now = func() time.Time { return unix(now - 100*86400) }
	team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic", Enabled: true, MemberIDs: []int64{user}})
	if err != nil {
		t.Fatal(err)
	}
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic", TeamID: team.ID, GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(conn)
	for _, item := range []struct {
		id    string
		at    int64
		known bool
	}{{"synthetic-old", now - 100*86400, true}, {"synthetic-recent", now, true}, {"synthetic-debt", now - 100*86400, false}} {
		if err = Begin(ctx, q, Request{ID: item.id, SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic", StartedAt: item.at}, item.at); err != nil {
			t.Fatal(err)
		}
		if err = Finish(ctx, q, item.id, Completion{Input: 1, Known: item.known, Dispatched: true}, item.at*1000, false); err != nil {
			t.Fatal(err)
		}
	}
	if err = Prune(ctx, q, now-90*86400); err != nil {
		t.Fatal(err)
	}
	if _, err = q.GetAllocationEntry(ctx, "synthetic-old"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("old entry retained", err)
	}
	for _, id := range []string{"synthetic-recent", "synthetic-debt"} {
		if _, err = q.GetAllocationEntry(ctx, id); err != nil {
			t.Fatal("lost balance or debt", id, err)
		}
	}
}

func TestPersonalReportFiltersBeforePendingLimit(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	if _, err := conn.Exec("INSERT INTO users(id,username,role,password_hash,enabled,created_at) VALUES(3,'synthetic-other','member','synthetic',1,1)"); err != nil {
		t.Fatal(err)
	}
	team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic", Enabled: true, MemberIDs: []int64{user, 3}})
	if err != nil {
		t.Fatal(err)
	}
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic", TeamID: team.ID, GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}, {UserID: 3, Limit: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	rev, err := Current(ctx, db.New(conn), scheme.ID, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 257; i++ {
		uid := int64(3)
		if i == 256 {
			uid = user
		}
		if _, err = conn.Exec("INSERT INTO allocation_entries(request_id,scheme_id,revision_id,user_id,account_id,model,mode,window_start,reset_at,started_at,state) VALUES(?,?,?,?,?,'synthetic','tokens',0,1,?,'pending')", fmt.Sprintf("synthetic-%03d", i), scheme.ID, rev.ID, uid, account, i); err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.Own(ctx, user)
	if err != nil || len(result) != 1 {
		t.Fatal(result, err)
	}
	report := result[0]
	if len(report.Pending) != 1 || report.Pending[0].UserID != user {
		t.Fatal("own pending debt hidden by other members", report.Pending)
	}
	if len(report.Config.Members) != 1 || report.Config.Members[0].UserID != user || len(report.Balances) != 1 || report.Balances[0].UserID != user {
		t.Fatal("another member exposed", report)
	}
}

func TestManagedAccountDeletionExplainsPoolLock(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic", Enabled: true, MemberIDs: []int64{user}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic", TeamID: team.ID, GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = accounts.New(conn, nil).Delete(ctx, account); !errors.Is(err, accounts.ErrAllocated) {
		t.Fatal("missing actionable lock error", err)
	}
}

func TestTeamGroupsAreTheOnlyMemberGrants(t *testing.T) {
	s, conn, user, _ := fixture(t)
	ctx := context.Background()
	pools := groups.New(conn)
	team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic access", Enabled: true, MemberIDs: []int64{user}, GroupIDs: []int64{2}})
	if err != nil {
		t.Fatal(err)
	}
	choices, err := pools.Available(ctx, user)
	if err != nil || len(choices) != 1 || choices[0].ID != 2 {
		t.Fatalf("team grants: %+v %v", choices, err)
	}
	all, err := s.Teams(ctx)
	if err != nil || len(all[0].GroupIDs) != 1 || all[0].GroupIDs[0] != 2 {
		t.Fatal(all, err)
	}
	second, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic second", Enabled: true, MemberIDs: []int64{user}, GroupIDs: []int64{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	choices, err = pools.Available(ctx, user)
	if err != nil || len(choices) != 2 {
		t.Fatalf("union not deduplicated: %+v %v", choices, err)
	}
	for _, ids := range [][]int64{{999}, {2, 2}} {
		if _, err := s.SaveTeam(ctx, team.ID, TeamInput{Name: team.Name, Enabled: true, MemberIDs: []int64{user}, GroupIDs: ids}); !errors.Is(err, ErrInput) {
			t.Fatal("invalid group IDs accepted", err)
		}
	}
	second.Enabled = false
	if _, err := s.SaveTeam(ctx, second.ID, second.TeamInput); err != nil {
		t.Fatal(err)
	}
	choices, err = pools.Available(ctx, user)
	if err != nil || len(choices) != 1 || choices[0].ID != 2 {
		t.Fatal("disabled team retained access", choices, err)
	}
	team.MemberIDs = []int64{}
	if _, err := s.SaveTeam(ctx, team.ID, team.TeamInput); err != nil {
		t.Fatal(err)
	}
	choices, err = pools.Available(ctx, user)
	if err != nil || len(choices) != 0 {
		t.Fatal("removal restored legacy grant", choices, err)
	}
}

func TestReservedPoolDoesNotInheritAnotherTeamsGroupGrant(t *testing.T) {
	s, conn, user, _ := fixture(t)
	ctx := context.Background()
	team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic prior access", Enabled: true, MemberIDs: []int64{user}, GroupIDs: []int64{2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("INSERT INTO users(id,username,role,password_hash,enabled,created_at) VALUES(3,'synthetic-other','member','synthetic',1,1)"); err != nil {
		t.Fatal(err)
	}
	owner, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic reservation", Enabled: true, MemberIDs: []int64{3}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic dedicated", TeamID: owner.ID, GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "month", Members: []Share{{UserID: 3, Limit: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	choices, err := groups.New(conn).Available(ctx, user)
	if err != nil || len(choices) != 0 {
		t.Fatal("other team's pool visible", choices, err)
	}
	if _, err := s.SaveTeam(ctx, team.ID, team.TeamInput); !errors.Is(err, ErrPoolConflict) {
		t.Fatal("reserved pool grant accepted", err)
	}
	choices, err = groups.New(conn).Available(ctx, 3)
	if err != nil || len(choices) != 1 || choices[0].ID != 2 {
		t.Fatal("own reserved pool missing", choices, err)
	}
}
