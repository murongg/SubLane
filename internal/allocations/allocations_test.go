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
	"github.com/murongg/SubLane/internal/pricing"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/tenants"
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
	if _, err = a.Setup(ctx, "synthetic-admin", "synthetic-pass", "Synthetic workspace"); err != nil {
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
	if _, err := groups.New(conn).Save(ctx, 0, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	pool, err := groups.New(conn).Save(ctx, 0, groups.Input{Name: "Synthetic pool", Enabled: true, AccountIDs: []string{account.ID}})
	if err != nil || pool.ID != 2 {
		t.Fatal(pool, err)
	}
	if _, err = groups.New(conn).Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	if err := groups.New(conn).SetMemberGroups(ctx, u.ID, []int64{2}); err != nil {
		t.Fatal(err)
	}
	return New(conn), conn, u.ID, account.ID
}

func grantPoolMembers(ctx context.Context, conn *sql.DB, poolID int64, members ...int64) error {
	for _, member := range members {
		if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO memberships(tenant_id,user_id,role,created_at)
			VALUES(1,?,'member',1)`, member); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, "INSERT OR IGNORE INTO group_members(group_id,user_id) VALUES(?,?)", poolID, member); err != nil {
			return err
		}
	}
	return nil
}

func TestAllocationSchemesStayInWorkspace(t *testing.T) {
	first, conn, member, _ := fixture(t)
	ctx := context.Background()
	if _, err := conn.Exec(`INSERT INTO users(id,username,role,password_hash,enabled,created_at)
		VALUES(3,'synthetic-other-owner','member','synthetic-hash',1,1);
		INSERT INTO tenants(id,name,owner_user_id,created_at) VALUES(2,'Other workspace',3,1);
		INSERT INTO memberships(tenant_id,user_id,role,created_at) VALUES(2,3,'owner',1);
		INSERT INTO accounts(id,tenant_id,provider,name,account_id,status,credential,created_at,updated_at)
		VALUES('other-account',2,'codex','Other account','other-subject','ready',x'01',1,1)`); err != nil {
		t.Fatal(err)
	}
	otherPool, err := groups.NewForTenant(conn, 2).Save(ctx, 0, groups.Input{Name: "Other pool", Enabled: true, AccountIDs: []string{"other-account"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := groups.NewForTenant(conn, 2).SetMemberGroups(ctx, 3, []int64{otherPool.ID}); err != nil {
		t.Fatal(err)
	}
	second := NewForTenant(conn, 2)
	firstScheme, err := first.SaveScheme(ctx, 0, SchemeInput{Name: "First allowance", GroupID: 2, Enabled: true,
		Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: member, Limit: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	secondScheme, err := second.SaveScheme(ctx, 0, SchemeInput{Name: "Second allowance", GroupID: otherPool.ID, Enabled: true,
		Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: 3, Limit: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Refresh(ctx, firstScheme.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign workspace allocation refreshed: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO allocation_windows(scheme_id,account_id,kind,reset_at,account_revision,observed_at,observed_points,baseline_points,unassigned)
		VALUES(?,'synthetic-account','primary',2000000000,0,1,0,0,5)`, firstScheme.ID); err != nil {
		t.Fatal(err)
	}
	if err := second.ReserveUnassigned(ctx, firstScheme.ID, 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign workspace allowance was reconciled: %v", err)
	}
	for _, check := range []struct {
		service *Service
		own     int64
		other   int64
	}{{first, firstScheme.ID, secondScheme.ID}, {second, secondScheme.ID, firstScheme.ID}} {
		all, err := check.service.Schemes(ctx)
		if err != nil || len(all) != 1 || all[0].ID != check.own {
			t.Fatalf("allocation list leaked across workspaces: %+v, %v", all, err)
		}
		if _, err := check.service.Detail(ctx, check.other, 0); !errors.Is(err, ErrNotFound) {
			t.Fatalf("foreign workspace allocation was readable: %v", err)
		}
	}
}

func TestPoolAllocationUsesDirectMemberGrant(t *testing.T) {
	s, conn, user, _ := fixture(t)
	ctx := context.Background()
	if err := groups.New(conn).SetMemberGroups(ctx, user, []int64{2}); err != nil {
		t.Fatal(err)
	}
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic direct allocation", GroupID: 2, Enabled: true,
		Config: Config{Mode: "tokens", Period: "month", Members: []Share{{UserID: user, Limit: 100}}}})
	if err != nil {
		t.Fatalf("pool allocation still requires a personnel team: %v", err)
	}
	if _, err := Authorize(ctx, db.New(conn), scheme.ID, user, 2, time.Now().Unix()); err != nil {
		t.Fatalf("directly granted member could not use the allocation: %v", err)
	}
}

func TestWorkspaceOwnerCanReceiveAllowanceWithoutPoolGrant(t *testing.T) {
	service, conn, _, _ := fixture(t)
	ctx := context.Background()
	var grants int
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM group_members WHERE group_id=2 AND user_id=1").Scan(&grants); err != nil || grants != 0 {
		t.Fatalf("owner unexpectedly has a direct grant: %d, %v", grants, err)
	}
	scheme, err := service.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic owner allowance", GroupID: 2, Enabled: true,
		Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: 1, Limit: 100}}}})
	if err != nil || len(scheme.Config.Members) != 1 || scheme.Config.Members[0].UserID != 1 {
		t.Fatalf("owner allowance: %+v, %v", scheme, err)
	}
	if _, err := Authorize(ctx, db.New(conn), scheme.ID, 1, scheme.GroupID, scheme.EffectiveAt); err != nil {
		t.Fatalf("owner could not use the assigned allowance: %v", err)
	}
}

func TestPlatformOwnerCanReceiveAllowanceAsMemberOfAnotherWorkspace(t *testing.T) {
	_, conn, ownerID, _ := fixture(t)
	ctx := context.Background()
	workspace, err := tenants.New(conn).Create(ctx, ownerID, "Second workspace")
	if err != nil {
		t.Fatal(err)
	}
	if err := tenants.New(conn).AddMember(ctx, ownerID, workspace.ID, 1, tenants.RoleMember); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO accounts(id,tenant_id,provider,name,account_id,status,credential,created_at,updated_at)
		VALUES('second-account',?,'codex','Synthetic account','synthetic-subject','ready',x'01',1,1)`, workspace.ID); err != nil {
		t.Fatal(err)
	}
	pools := groups.NewForTenant(conn, workspace.ID)
	pool, err := pools.Save(ctx, 0, groups.Input{Name: "Second pool", Enabled: true, AccountIDs: []string{"second-account"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := pools.SetMemberGroups(ctx, 1, []int64{pool.ID}); err != nil {
		t.Fatal(err)
	}
	scheme, err := NewForTenant(conn, workspace.ID).SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic allowance", GroupID: pool.ID, Enabled: true,
		Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: 1, Limit: 100}}}})
	if err != nil || len(scheme.Config.Members) != 1 || scheme.Config.Members[0].UserID != 1 {
		t.Fatalf("workspace member 1 could not receive allowance: %+v, %v", scheme, err)
	}
}

func TestTeamsSchemesAndExclusiveCapacity(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	input := SchemeInput{Name: "Synthetic scheme", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "month", Members: []Share{{UserID: user, Limit: 1500000}}, Rates: []Rate{}}}
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

func TestSchemeCopiesAutomaticPriceIntoRevision(t *testing.T) {
	_, conn, user, _ := fixture(t)
	ctx := context.Background()
	manager := NewWithPricing(conn, pricing.NewStatic(map[string]pricing.Price{
		"gpt-5.1-codex": {Input: 1250000, Cached: 125000, Output: 10000000},
	}))
	scheme, err := manager.SaveScheme(ctx, 0, SchemeInput{Name: "Codex shared plan", GroupID: 2, Enabled: true, Config: Config{Mode: "amount", Period: "day", Members: []Share{{UserID: user, Limit: 100}}, Rates: []Rate{{Model: "gpt-5.1-codex"}}}})
	if err != nil {
		t.Fatal(err)
	}
	rate := scheme.Config.Rates[0]
	if rate.Input != 1250000 || rate.Cached != 125000 || rate.Output != 10000000 {
		t.Fatalf("automatic price not snapshotted: %+v", rate)
	}
}

func TestRatioSchemeBuildsRatesFromPoolCatalog(t *testing.T) {
	_, conn, user, account := fixture(t)
	ctx := context.Background()
	if _, err := conn.Exec(`UPDATE accounts SET models_snapshot = ? WHERE id = ?`,
		`{"models":["gpt-5.1-codex","claude-3-7-sonnet-20250219"],"updated_at":1,"source":"synthetic"}`, account); err != nil {
		t.Fatal(err)
	}
	manager := NewWithPricing(conn, pricing.NewStatic(map[string]pricing.Price{
		"gpt-5.1-codex":              {Input: 1250000, Cached: 125000, Output: 10000000},
		"claude-3-7-sonnet-20250219": {Input: 3000000, Cached: 300000, Output: 15000000},
	}))
	scheme, err := manager.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic ratio", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", Period: "upstream", Members: []Share{{UserID: user, Limit: 10000}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(scheme.Config.Rates) != 2 || scheme.Config.Rates[0].Model != "claude-3-7-sonnet-20250219" || scheme.Config.Rates[1].Model != "gpt-5.1-codex" {
		t.Fatalf("automatic ratio rates not snapshotted: %+v", scheme.Config.Rates)
	}
}

func TestRatioSchemeRejectsUnpricedPoolModel(t *testing.T) {
	_, conn, user, account := fixture(t)
	ctx := context.Background()
	if _, err := conn.Exec(`UPDATE accounts SET models_snapshot = ? WHERE id = ?`,
		`{"models":["unknown-codex-model"],"updated_at":1,"source":"synthetic"}`, account); err != nil {
		t.Fatal(err)
	}
	manager := NewWithPricing(conn, pricing.NewStatic(map[string]pricing.Price{}))
	_, err := manager.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic ratio", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", Period: "upstream", Members: []Share{{UserID: user, Limit: 10000}}}})
	if !errors.Is(err, ErrUnpriced) {
		t.Fatalf("want unpriced model error, got %v", err)
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
	input := SchemeInput{Name: "Synthetic scheme", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "month", Members: []Share{{UserID: user, Limit: 100}}}}
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
	now := int64(1_900_000_000)
	// Scheme activation and admission must share a clock even when CI crosses a second.
	s.now = func() time.Time { return unix(now) }
	q := db.New(conn)
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic budget", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 5}}}})
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
	if err = q.RecoverAllocationEntries(ctx, 1); err != nil {
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
	in := SchemeInput{Name: "Synthetic", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", Period: "upstream", Members: []Share{{UserID: user, Limit: 10000}}, Rates: []Rate{{Model: "synthetic", Input: 1, Cached: 1, Output: 1}}}}
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

func TestFirstPoolCanBeReserved(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	pools := groups.New(conn)
	if _, err := pools.Save(ctx, 2, groups.Input{Name: "Synthetic pool", Enabled: true, AccountIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := pools.Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{account}}); err != nil {
		t.Fatal(err)
	}
	if err := pools.SetMemberGroups(ctx, user, []int64{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic", GroupID: 1, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}}); err != nil {
		t.Fatal("first pool cannot be reserved", err)
	}
	cipher, err := vault.Open(filepath.Join(t.TempDir(), "synthetic-import-key"), true)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := accounts.New(conn, cipher).Authorize(ctx, "Synthetic later import", accounts.Credential{AccountID: "synthetic-later", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal("reserving the first pool blocked later imports", err)
	}
	var count int
	if err = conn.QueryRow("SELECT count(*) FROM group_accounts WHERE account_id=?", imported.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("later import entered a reserved pool", err)
	}

}

func TestTokenCorrectionRejectsOversizedUsage(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(conn)
	now := time.Now().Unix()
	if err = Begin(ctx, q, Request{ID: "synthetic-large", SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic", StartedAt: now}, now); err != nil {
		t.Fatal(err)
	}
	if err = q.RecoverAllocationEntries(ctx, 1); err != nil {
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
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}})
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
	if err := grantPoolMembers(ctx, conn, 2, 3); err != nil {
		t.Fatal(err)
	}
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}, {UserID: 3, Limit: 100}}}})
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
	_, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = accounts.New(conn, nil).Delete(ctx, account); !errors.Is(err, accounts.ErrAllocated) {
		t.Fatal("missing actionable lock error", err)
	}
}
