package gateway

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/groups"
)

func budgetRule(t *testing.T, s *Service, user, group int64, model, period string, limit int64) Budget {
	t.Helper()
	b, err := s.SaveBudget(context.Background(), user, BudgetInput{GroupID: group, Model: model, Period: period, Limit: limit, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func budgetCall(t *testing.T, s *Service, user, group int64, model string) *Exchange {
	t.Helper()
	x, err := s.Open(context.Background(), user, group, []byte(`{"model":"`+model+`","input":"synthetic"}`), nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	return x
}
func budgetFinish(t *testing.T, x *Exchange) {
	t.Helper()
	if err := x.Events(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	x.Body.Close()
	x.Body.Close()
}
func TestBudgetsScopesAliasesAndEdits(t *testing.T) {
	ctx := context.Background()
	s, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	user := runtimeMember(t, s)
	pool, err := groups.New(s.db).Save(ctx, 0, groups.Input{Name: "Synthetic pool", Enabled: true, AccountIDs: []string{ids["codex"]}})
	if err != nil {
		t.Fatal(err)
	}
	if err := groups.New(s.db).SetMemberGroups(ctx, user, []int64{1, pool.ID}); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []struct {
		group int64
		model string
	}{{0, ""}, {1, ""}, {0, "synthetic-model"}, {1, "synthetic-model"}} {
		budgetRule(t, s, user, scope.group, scope.model, "month", 100)
	}
	daily := budgetRule(t, s, user, 1, "codex/synthetic-model", "day", 5)
	budgetFinish(t, budgetCall(t, s, user, 1, "synthetic-model")) // 3 input + 2 output, cached is a subset.
	budgets, err := s.Budgets(ctx, user)
	if err != nil || len(budgets.Rules) != 5 {
		t.Fatal(budgets, err)
	}
	for _, b := range budgets.Rules {
		if b.Used != 5 {
			t.Fatal("matching scope was not charged exactly once", b)
		}
	}
	if _, err = s.Open(ctx, user, 1, []byte(`{"model":"codex/synthetic-model","input":[]}`), nil, Responses); !errors.Is(err, ErrTokenQuota) {
		t.Fatal("alias bypass", err)
	}
	budgetFinish(t, budgetCall(t, s, user, pool.ID, "synthetic-model"))
	budgetFinish(t, budgetCall(t, s, 1, 1, "synthetic-model")) // other user unaffected
	daily, err = s.SaveBudget(ctx, user, BudgetInput{GroupID: 1, Model: "synthetic-model", Period: "day", Limit: 10, Enabled: true})
	if err != nil || daily.Used != 5 {
		t.Fatal("edit reset usage", daily, err)
	}
	budgetFinish(t, budgetCall(t, s, user, 1, "synthetic-model"))
	daily, err = s.SaveBudget(ctx, user, BudgetInput{GroupID: 1, Model: "synthetic-model", Period: "day", Limit: 1, Enabled: false})
	if err != nil || daily.Used != 10 {
		t.Fatal(daily, err)
	}
	budgetFinish(t, budgetCall(t, s, user, 1, "synthetic-model"))
	daily, err = s.SaveBudget(ctx, user, BudgetInput{GroupID: 1, Model: "synthetic-model", Period: "day", Limit: 5, Enabled: true})
	if err != nil || daily.Used != 15 {
		t.Fatal("reenabling lost usage", daily, err)
	}
}
func TestBudgetsPendingSettlementAndRestart(t *testing.T) {
	ctx := context.Background()
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	user := runtimeMember(t, s)
	budgetRule(t, s, user, 0, "", "day", 100)
	budgetRule(t, s, user, 1, "synthetic-model", "month", 100)
	x := budgetCall(t, s, user, 1, "synthetic-model")
	x.Body.Close() // no final usage observed
	page, err := s.Budgets(ctx, user)
	if err != nil || len(page.Pending) != 1 || page.Rules[0].Pending != 1 {
		t.Fatal(page, err)
	}
	restarted := New(ctx, s.db, s.accounts, s.provider)
	defer restarted.Close()
	if _, err := restarted.Open(ctx, user, 1, []byte(`{"model":"synthetic-model","input":[]}`), nil, Responses); !errors.Is(err, ErrTokenPending) {
		t.Fatal("unknown usage admitted", err)
	}
	req := page.Pending[0].RequestID
	if err := restarted.ResolveBudget(ctx, user+1, req, 7); err == nil {
		t.Fatal("cross-owner settlement")
	}
	if err := restarted.ResolveBudget(ctx, user, req, 7); err != nil {
		t.Fatal(err)
	}
	if err := restarted.ResolveBudget(ctx, user, req, 7); err != nil {
		t.Fatal("idempotent retry", err)
	}
	if err := restarted.ResolveBudget(ctx, user, req, 8); err == nil {
		t.Fatal("conflicting retry accepted")
	}
	page, err = restarted.Budgets(ctx, user)
	if err != nil || len(page.Pending) != 0 {
		t.Fatal(page, err)
	}
	for _, b := range page.Rules {
		if b.Used != 7 || b.Pending != 0 {
			t.Fatal("manual settlement missed scope", b)
		}
	}
	budgetFinish(t, budgetCall(t, restarted, user, 1, "synthetic-model"))
}
func TestBudgetsRolloverKeepsOriginalWindow(t *testing.T) {
	ctx := context.Background()
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	user := runtimeMember(t, s)
	now := time.Now().UTC()
	end := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	current := end.Add(-time.Second)
	s.now = func() time.Time { return current }
	budgetRule(t, s, user, 0, "", "day", 5)
	budgetRule(t, s, user, 0, "", "month", 5)
	x := budgetCall(t, s, user, 1, "synthetic-model")
	current = end.Add(time.Second)
	budgetFinish(t, x)
	page, err := s.Budgets(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range page.Rules {
		if b.Used != 0 {
			t.Fatal("charged completion window", b)
		}
	}
	budgetFinish(t, budgetCall(t, s, user, 1, "synthetic-model"))
	if _, err := s.Open(ctx, user, 1, []byte(`{"model":"synthetic-model","input":[]}`), nil, Responses); !errors.Is(err, ErrTokenQuota) {
		t.Fatal(err)
	}
}
func TestBudgetsConcurrentSettlementAndStorageFailure(t *testing.T) {
	ctx := context.Background()
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	user := runtimeMember(t, s)
	budgetRule(t, s, user, 0, "", "day", 5)
	first := budgetCall(t, s, user, 1, "synthetic-model")
	second := budgetCall(t, s, user, 1, "synthetic-model")
	var wg sync.WaitGroup
	for _, x := range []*Exchange{first, second} {
		wg.Go(func() { budgetFinish(t, x) })
	}
	wg.Wait()
	page, err := s.Budgets(ctx, user)
	if err != nil || page.Rules[0].Used != 10 {
		t.Fatal("lost concurrent usage", page, err)
	}
	budgetRule(t, s, user, 0, "", "day", 100)
	if _, err := s.db.Exec("CREATE TRIGGER fail_budget BEFORE UPDATE ON token_budget_usage BEGIN SELECT RAISE(ABORT,'synthetic failure'); END"); err != nil {
		t.Fatal(err)
	}
	budgetFinish(t, budgetCall(t, s, user, 1, "synthetic-model"))
	if _, err := s.Open(ctx, user, 1, []byte(`{"model":"synthetic-model","input":[]}`), nil, Responses); !errors.Is(err, ErrTokenAccounting) {
		t.Fatal("accounting failure opened quota", err)
	}
	if _, err := s.db.Exec("DROP TRIGGER fail_budget"); err != nil {
		t.Fatal(err)
	}
	restarted := New(ctx, s.db, s.accounts, s.provider)
	defer restarted.Close()
	page, err = restarted.Budgets(ctx, user)
	if err != nil || len(page.Pending) != 1 {
		t.Fatal("uncommitted settlement lost after restart", page, err)
	}
}
func TestBudgetsValidationAndLocalFailure(t *testing.T) {
	ctx := context.Background()
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	user := runtimeMember(t, s)
	for _, input := range []BudgetInput{{Period: "week", Limit: 1}, {Period: "day", Limit: 0}, {Period: "day", Limit: -1}, {Period: "day", Limit: 1, GroupID: 999}, {Period: "day", Limit: 1, Model: "*"}} {
		if _, err := s.SaveBudget(ctx, user, input); err == nil {
			t.Fatal("invalid rule accepted", input)
		}
	}
	budgetRule(t, s, user, 0, "", "day", 1)
	if _, err := s.Open(ctx, user, 1, []byte(`{"model":"missing-model","input":[]}`), nil, Responses); err == nil {
		t.Fatal("unsupported model admitted")
	}
	page, err := s.Budgets(ctx, user)
	if err != nil || page.Rules[0].Used != 0 || len(page.Pending) != 0 {
		t.Fatal("local rejection created debt", page, err)
	}
}

func TestBudgetsPartialUsageStaysPendingAcrossPeriodsAndKeepsOtherScopesOpen(t *testing.T) {
	ctx := context.Background()
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	user := runtimeMember(t, s)
	budgetRule(t, s, user, 1, "synthetic-model", "day", 100)
	x := budgetCall(t, s, user, 1, "synthetic-model")
	known := int64(3)
	x.entry.record.InputTokens = &known
	x.Body.Close()
	page, err := s.Budgets(ctx, user)
	if err != nil || page.Rules[0].Used != 3 || page.Pending[0].KnownTokens != 3 {
		t.Fatal(page, err)
	}
	if err := s.ResolveBudget(ctx, user, page.Pending[0].RequestID, 2); !errors.Is(err, ErrBudgetSettlement) {
		t.Fatal("allowed lower than observed usage", err)
	}
	next := s.now().AddDate(0, 1, 0)
	s.now = func() time.Time { return next }
	page, err = s.Budgets(ctx, user)
	if err != nil || page.Rules[0].Used != 0 || page.Rules[0].Pending != 1 {
		t.Fatal("rollover discarded debt", page, err)
	}
	// Test the shared admission directly, avoiding expired catalog fixtures after a simulated month.
	e, err := s.begin(ctx, user, 1, Responses)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	err = s.admitBudget(ctx, e, "synthetic-model")
	s.mu.Unlock()
	e.fail(err)
	if !errors.Is(err, ErrTokenPending) {
		t.Fatal("rollover bypassed pending usage", err)
	}
	e, err = s.begin(ctx, user, 1, Responses)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	err = s.admitBudget(ctx, e, "other-synthetic-model")
	s.mu.Unlock()
	e.fail(err)
	if err != nil {
		t.Fatal("unrelated model blocked", err)
	}
	if err := s.ResolveBudget(ctx, user, page.Pending[0].RequestID, 7); err != nil {
		t.Fatal(err)
	}
	page, err = s.Budgets(ctx, user)
	if err != nil || page.Rules[0].Used != 0 || page.Rules[0].Pending != 0 {
		t.Fatal("late settlement charged new period", page, err)
	}
}

func TestBudgetsAllTransportsUseAdmissionBeforeUpstream(t *testing.T) {
	ctx := context.Background()
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	user := runtimeMember(t, s)
	budgetRule(t, s, user, 0, "synthetic-model", "day", 5)
	budgetFinish(t, budgetCall(t, s, user, 1, "synthetic-model"))
	for _, v := range []struct {
		kind Kind
		raw  string
	}{
		{Responses, `{"model":"synthetic-model","input":[]}`},
		{Compact, `{"model":"synthetic-model","input":[]}`},
		{Chat, `{"model":"synthetic-model","messages":[{"role":"user","content":"synthetic"}]}`},
		{Messages, `{"model":"synthetic-model","max_tokens":100,"messages":[{"role":"user","content":"synthetic"}]}`},
		{Gemini, `{"model":"synthetic-model","contents":[{"role":"user","parts":[{"text":"synthetic"}]}]}`},
		{GeminiStream, `{"model":"synthetic-model","contents":[{"role":"user","parts":[{"text":"synthetic"}]}]}`},
	} {
		if _, err := s.Open(ctx, user, 1, []byte(v.raw), nil, v.kind); !errors.Is(err, ErrTokenQuota) {
			t.Fatalf("kind %v bypassed budget: %v", v.kind, err)
		}
	}
}
