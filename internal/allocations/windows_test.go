package allocations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage/db"
)

func TestRatioDurationWindowsChargeBothWithoutUpstreamQuota(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	now := int64(1_900_000_000)
	s.now = func() time.Time { return time.Unix(now, 0) }
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic window shares", GroupID: 2, Enabled: true,
		Config: Config{Mode: "ratio", RatioUnit: "tokens", Period: "durations", Windows: []WindowCondition{{DurationSeconds: 5 * 3600, Limit: 100}, {DurationSeconds: 7 * 86400, Limit: 400}}, Members: []Share{{UserID: user, Limit: 5000}}}})
	if err != nil {
		t.Fatal("duration shares rejected", err)
	}
	q := db.New(conn)
	r := Request{ID: "synthetic-window-request", SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic-model", StartedAt: now}
	if err := Begin(ctx, q, r, now, time.UTC); err != nil {
		t.Fatal(err)
	}
	if err := Finish(ctx, q, r.ID, Completion{Input: 50, Known: true, Dispatched: true}, now*1000, false); err != nil {
		t.Fatal(err)
	}
	detail, err := s.Detail(ctx, scheme.ID, user)
	if err != nil || len(detail.Balances) != 2 || detail.Balances[0].Limit != 50 || detail.Balances[1].Limit != 200 || detail.Balances[0].Used != 50 || detail.Balances[1].Used != 50 {
		t.Fatal("incorrect window balances", detail.Balances, err)
	}
	if _, err := Check(ctx, q, scheme.ID, user, 2, account, r.Model, now, time.UTC); !errors.Is(err, ErrQuota) {
		t.Fatal("short quota not enforced", err)
	}
	now += 5 * 3600
	detail, err = s.Detail(ctx, scheme.ID, user)
	if err != nil || detail.Balances[0].Used != 0 || detail.Balances[1].Used != 50 {
		t.Fatal("short renewal cleared longer usage", detail.Balances, err)
	}
}

func TestScheduledBudgetEditPreservesDurationAnchorAndUsage(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	start := int64(1_900_000_000)
	now := start
	s.now = func() time.Time { return time.Unix(now, 0) }
	in := SchemeInput{Name: "Synthetic anchored budget", GroupID: 2, Enabled: true, Config: Config{Mode: "windows", Period: "durations", Windows: []WindowCondition{{DurationSeconds: 5 * 3600, Limit: 100}, {DurationSeconds: 7 * 86400, Limit: 1000}}, Members: []Share{{UserID: user}}, Rates: []Rate{{Model: "synthetic", Input: 1_000_000, Output: 1_000_000}}}}
	scheme, err := s.SaveScheme(ctx, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	now += 167 * 3600
	q := db.New(conn)
	r := Request{ID: "synthetic-before-edit", SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic", StartedAt: now}
	if err := Begin(ctx, q, r, now, time.UTC); err != nil {
		t.Fatal(err)
	}
	if err := Finish(ctx, q, r.ID, Completion{Input: 40, Known: true, Dispatched: true}, now*1000, false); err != nil {
		t.Fatal(err)
	}
	in.Config.Windows[0].Limit = 80
	updated, err := s.SaveScheme(ctx, scheme.ID, conditioned(t, s, scheme.ID, in))
	if err != nil || updated.Next == nil {
		t.Fatal(updated, err)
	}
	now = start + 168*3600
	detail, err := s.Detail(ctx, scheme.ID, user)
	if err != nil || detail.Balances[0].Used != 40 || detail.Balances[0].Limit != 80 || detail.Balances[0].ResetAt != start+170*3600 || detail.Balances[1].Used != 0 {
		t.Fatal("scheduled values restarted a short interval", detail.Balances, err)
	}
}

func TestSharedBudgetIncludesFormerRecipients(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	now := int64(1_900_000_000)
	s.now = func() time.Time { return time.Unix(now, 0) }
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic shared total", GroupID: 2, Enabled: true,
		Config: Config{Mode: "ratio", RatioUnit: "tokens", Total: 100, Period: "month", Members: []Share{{UserID: user, Limit: 10000}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO allocation_entries(request_id,scheme_id,revision_id,user_id,account_id,model,mode,window_start,reset_at,started_at,state,cost)
		VALUES('synthetic-former-charge',?,?,1,?,'synthetic-model','tokens',?, ?,?,'settled',100)`, scheme.ID, scheme.Revision.ID, account, now, now+86400, now); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(ctx, db.New(conn), scheme.ID, user, 2, account, "synthetic-model", now, time.UTC); !errors.Is(err, ErrQuota) {
		t.Fatal("former recipient escaped shared total", err)
	}
	detail, err := s.Detail(ctx, scheme.ID, user)
	if err != nil || detail.Balances[0].Admission != "exhausted" {
		t.Fatal("report disagrees with shared admission", detail.Balances, err)
	}
	if !detail.Balances[0].SharedBudgetBlocked {
		t.Fatal("positive personal balance lacks the shared blocking reason")
	}
}

func TestRatioAmountWindowsUseCapturedPricesAndRetainLongerUsage(t *testing.T) {
	s, conn, user, account := fixture(t)
	now := syntheticClock(s)
	ctx := context.Background()
	in := SchemeInput{Name: "Synthetic priced window shares", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", RatioUnit: "amount", Period: "durations", Windows: []WindowCondition{{DurationSeconds: 5 * 3600, Limit: 100}, {DurationSeconds: 7 * 86400, Limit: 400}}, Members: []Share{{UserID: user, Limit: 5000}}, Rates: []Rate{{Model: "synthetic-model", Input: 2_000_000, Cached: 1_000_000, Output: 3_000_000}}}}
	out, err := s.SaveScheme(ctx, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(conn)
	r := Request{ID: "synthetic-priced-window", SchemeID: out.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic-model", StartedAt: *now}
	if err := Begin(ctx, q, r, *now, time.UTC); err != nil {
		t.Fatal(err)
	}
	if err := Finish(ctx, q, r.ID, Completion{Input: 20, Cached: 10, Output: 10, Known: true, Dispatched: true}, *now*1000, false); err != nil {
		t.Fatal(err)
	}
	detail, err := s.Detail(ctx, out.ID, user)
	if err != nil || len(detail.Balances) != 2 {
		t.Fatal(detail, err)
	}
	for _, balance := range detail.Balances {
		if balance.Mode != "amount" || balance.Used != 60 || balance.Tokens != 30 {
			t.Fatal("cached input was counted twice or saved rates were ignored", balance)
		}
	}
	if _, err := Check(ctx, q, out.ID, user, 2, account, r.Model, *now, time.UTC); err != ErrQuota {
		t.Fatal("short amount window failed to block after a soft-cap overage", err)
	}
	*now += 5 * 3600
	detail, err = s.Detail(ctx, out.ID, user)
	if err != nil || detail.Balances[0].Used != 0 || detail.Balances[1].Used != 60 {
		t.Fatal("amount window renewal cleared the longer balance", detail.Balances, err)
	}
}
