package gateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/upstream"
)

func TestRatioAutomaticallyReconcilesAfterLastRequest(t *testing.T) {
	f := newQuotaFixture(t)
	s := f.service
	s.now = func() time.Time { return time.Unix(f.clock.Load(), 0) }
	ctx := context.Background()
	a, _ := auth.New(f.connection)
	if _, err := a.Setup(ctx, "synthetic-admin", "synthetic-password"); err != nil {
		t.Fatal(err)
	}
	member, err := a.CreateMember(ctx, "synthetic-member", "synthetic-password")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := groups.New(s.db).Save(ctx, 0, groups.Input{Name: "Synthetic pool", Enabled: true, AccountIDs: []string{f.id}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := groups.New(s.db).Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	manager := s.Allocations()
	team, err := manager.SaveTeam(ctx, 0, allocations.TeamInput{Name: "Synthetic team", Enabled: true, MemberIDs: []int64{member.ID}})
	if err != nil {
		t.Fatal(err)
	}
	scheme, err := manager.SaveScheme(ctx, 0, allocations.SchemeInput{Name: "Synthetic share", TeamID: team.ID, GroupID: pool.ID, Enabled: true, Config: allocations.Config{Mode: "ratio", Period: "upstream", Members: []allocations.Share{{UserID: member.ID, Limit: 10000}}, Rates: []allocations.Rate{{Model: "synthetic", Input: 1, Output: 1, Cached: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Store(time.Now().Unix())
	waitQuota(t, f)
	if err := manager.Refresh(ctx, scheme.ID); err != nil {
		t.Fatal(err)
	}
	s.provider = upstream.NewWithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"rate_limit":{"primary_window":{"used_percent":30,"limit_window_seconds":18000,"reset_at":2000000000}}}`))}, nil
	}))
	t.Cleanup(s.provider.Close)
	e, err := s.begin(ctx, member.ID, pool.ID, Responses)
	if err != nil {
		t.Fatal(err)
	}
	defer e.finish("canceled", "test_cleanup", "", "")
	e.schemeID, e.allocationTracked, e.budgetDispatched = scheme.ID, true, true
	e.record.AccountID, e.record.Model = f.id, "synthetic"
	in, out := int64(100), int64(10)
	e.record.InputTokens, e.record.OutputTokens = &in, &out
	if err := allocations.Begin(ctx, s.queries, allocations.Request{ID: e.record.RequestID, SchemeID: scheme.ID, UserID: member.ID, GroupID: pool.ID, AccountID: f.id, Model: "synthetic", StartedAt: s.now().Unix()}, s.now().Unix()); err != nil {
		t.Fatal(err)
	}
	e.finish("success", "", "", "")
	f.clock.Add(6)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		entry, err := s.queries.GetAllocationEntry(ctx, e.record.RequestID)
		if err != nil {
			t.Fatal(err)
		}
		if entry.State == "settled" {
			detail, err := manager.Detail(ctx, scheme.ID, member.ID)
			if err != nil || len(detail.Balances) != 1 || detail.Balances[0].Used != 500 {
				t.Fatal("wrong automatic debit", detail, err)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("last request never reconciled without another request or administrator refresh")
}

func TestAllocationKeyUsesOneModeAndSharedLedger(t *testing.T) {
	ctx := context.Background()
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	user := runtimeMember(t, s)
	ids, err := s.queries.ListGroupAccounts(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := groups.New(s.db).Save(ctx, 0, groups.Input{Name: "Synthetic pool", Enabled: true, AccountIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = groups.New(s.db).Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	manager := allocations.New(s.db)
	team, err := manager.SaveTeam(ctx, 0, allocations.TeamInput{Name: "Synthetic team", Enabled: true, MemberIDs: []int64{user}})
	if err != nil {
		t.Fatal(err)
	}
	scheme, err := manager.SaveScheme(ctx, 0, allocations.SchemeInput{Name: "Synthetic amount", TeamID: team.ID, GroupID: pool.ID, Enabled: true, Config: allocations.Config{Mode: "amount", Period: "day", Members: []allocations.Share{{UserID: user, Limit: 16}}, Rates: []allocations.Rate{{Model: "synthetic-model", Input: 2000000, Cached: 1000000, Output: 6000000}}}})
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic stream has 3 input (2 cached) and 2 output = 16 micro-USD.
	budgetRule(t, s, user, 0, "", "day", 1)
	// Bind synthetic hashed key rows through the same durable relation as personal keys.
	result, err := s.db.Exec("INSERT INTO api_keys(user_id,group_id,name,prefix,token_hash,created_at) VALUES(?,2,'synthetic','sl_fake',randomblob(32),1)", user)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := result.LastInsertId()
	if _, err = s.db.Exec("INSERT INTO allocation_keys(key_id,scheme_id) VALUES(?,?)", key, scheme.ID); err != nil {
		t.Fatal(err)
	}
	callctx := WithRequestIdentity(ctx, key, "http")
	x, err := s.Open(callctx, user, pool.ID, []byte(`{"model":"synthetic-model","input":[]}`), nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	budgetFinish(t, x)
	if _, err = s.Open(WithRequestIdentity(ctx, key, "websocket"), user, pool.ID, []byte(`{"model":"codex/synthetic-model","input":[]}`), nil, Responses); !errors.Is(err, allocations.ErrQuota) {
		t.Fatal("scheme did not enforce amount or stacked legacy token limit", err)
	}
	if _, err = s.Open(ctx, user, pool.ID, []byte(`{"model":"synthetic-model","input":[]}`), nil, Responses); !errors.Is(err, allocations.ErrUnavailable) {
		t.Fatal("unbound key bypassed scheme", err)
	}
}

func TestAllocationSyncRetainsCompletionDuringWorkerExit(t *testing.T) {
	s, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	s.mu.Lock()
	s.scheduleAllocationSync(1, ids["codex"])
	before := s.allocationSync[ids["codex"]]
	s.scheduleAllocationSync(1, ids["codex"])
	after := s.allocationSync[ids["codex"]]
	s.mu.Unlock()
	if before == after {
		t.Fatal("a completion while the worker exits must request another sync pass")
	}
	// Shutdown must cancel and join even a coalesced follow-up without adding a worker after Close.
	s.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.allocationSync) != 0 {
		t.Fatal("sync workers survived shutdown")
	}
}
