package allocations

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage/db"
)

func TestRatioObservedModelWeightsAndDelayedUsage(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	q := db.New(conn)
	now := time.Now().Unix()
	s.now = func() time.Time { return unix(now) }
	for _, id := range []int64{3, 4} {
		if _, err := conn.Exec("INSERT INTO users(id,username,role,password_hash,enabled,created_at) VALUES(?,?,'member','synthetic',1,1)", id, "synthetic-"+string(rune('a'+id))); err != nil {
			t.Fatal(err)
		}
	}
	team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic ratio team", Enabled: true, MemberIDs: []int64{user, 3, 4}})
	if err != nil {
		t.Fatal(err)
	}
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic ratio", TeamID: team.ID, GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", Period: "upstream", Members: []Share{{UserID: user, Limit: 5000}, {UserID: 3, Limit: 3000}, {UserID: 4, Limit: 2000}}, Rates: []Rate{{Model: "model-x", Input: 4000000, Cached: 4000000, Output: 4000000}, {Model: "model-y", Input: 1000000, Cached: 1000000, Output: 1000000}}}})
	if err != nil {
		t.Fatal(err)
	}
	reset := now + 1000
	snapshot := func(points int64) {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"updated_at": now, "read_started_at": now * 1000, "limits": []any{map[string]any{"name": "", "windows": []any{map[string]any{"kind": "primary", "used_percent": float64(points) / 100, "reset_at": reset}}}}})
		r, err := q.GetAccountUsage(ctx, account)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = q.SaveAccountUsage(ctx, db.SaveAccountUsageParams{AccountID: account, Snapshot: raw, UpdatedAt: now, Revision: r.Revision}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot(0)
	if err = s.Refresh(ctx, scheme.ID); err != nil {
		t.Fatal(err)
	}
	for i, uid := range []int64{user, 3, 4} {
		model := "model-y"
		if i == 0 {
			model = "model-x"
		}
		id := "synthetic-ratio-" + string(rune('a'+i))
		if err = Begin(ctx, q, Request{ID: id, SchemeID: scheme.ID, UserID: uid, GroupID: 2, AccountID: account, Model: model, StartedAt: now}, now); err != nil {
			t.Fatal(err)
		}
		if err = Finish(ctx, q, id, Completion{Input: 1000000, Known: true, Dispatched: true}, now*1000, false); err != nil {
			t.Fatal(err)
		}
	}
	now++
	snapshot(0)
	if err = s.Refresh(ctx, scheme.ID); err != nil {
		t.Fatal(err)
	}
	page, err := s.Detail(ctx, scheme.ID, 0)
	if err != nil || len(page.Pending) != 3 {
		t.Fatal("zero delta erased debt", page, err)
	}
	now++
	snapshot(1200)
	if err = s.Refresh(ctx, scheme.ID); err != nil {
		t.Fatal(err)
	}
	page, err = s.Detail(ctx, scheme.ID, 0)
	if err != nil || len(page.Pending) != 0 || len(page.Balances) != 3 {
		t.Fatal(page, err)
	}
	for i, want := range []int64{800, 200, 200} {
		if page.Balances[i].Used != want {
			t.Fatal("model weights were ignored", page.Balances)
		}
	}
	// A manually settled percentage must not be charged again by the next upstream observation.
	if err = Begin(ctx, q, Request{ID: "synthetic-manual", SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "model-y", StartedAt: now}, now); err != nil {
		t.Fatal(err)
	}
	if err = Finish(ctx, q, "synthetic-manual", Completion{Input: 1000000, Known: true, Dispatched: true}, now*1000, false); err != nil {
		t.Fatal(err)
	}
	debts, err := q.GetRequestAllocationDebits(ctx, "synthetic-manual")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Settle(ctx, scheme.ID, "synthetic-manual", Completion{}, map[int64]int64{debts[0].WindowID: 100}); err != nil {
		t.Fatal(err)
	}
	now++
	snapshot(1300)
	if err = s.Refresh(ctx, scheme.ID); err != nil {
		t.Fatal(err)
	}
	page, err = s.Detail(ctx, scheme.ID, 0)
	if err != nil || page.Unassigned != 0 || page.Balances[0].Used != 900 {
		t.Fatal("manual charge counted twice", page, err)
	}
	if _, err = Check(ctx, q, scheme.ID, user, 2, account, "missing", now); !errors.Is(err, ErrUnpriced) {
		t.Fatal("unpriced model admitted", err)
	}

	if err = Begin(ctx, q, Request{ID: "synthetic-interrupted", SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "model-y", StartedAt: now}, now); err != nil {
		t.Fatal(err)
	}
	if err = Finish(ctx, q, "synthetic-interrupted", Completion{Dispatched: true}, now*1000, false); err != nil {
		t.Fatal(err)
	}
	original, err := q.GetRequestAllocationDebits(ctx, "synthetic-interrupted")
	if err != nil {
		t.Fatal(err)
	}
	now = reset + 1
	reset = now + 1000
	snapshot(0)
	if err = s.Refresh(ctx, scheme.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = Check(ctx, q, scheme.ID, user, 2, account, "model-y", now); !errors.Is(err, ErrPending) {
		t.Fatal("reset erased unknown debt", err)
	}
	if err = s.Settle(ctx, scheme.ID, "synthetic-interrupted", Completion{}, map[int64]int64{original[0].WindowID: 100}); err != nil {
		t.Fatal(err)
	}
	page, err = s.Detail(ctx, scheme.ID, 0)
	if err != nil || page.Balances[0].Used != 0 {
		t.Fatal("old settlement charged new window", page, err)
	}
	if _, err = Check(ctx, q, scheme.ID, user, 2, account, "model-y", now); err != nil {
		t.Fatal(err)
	}
}

func TestRatioHealthyAccountSurvivesUnavailablePeerAndRevisionChange(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	q := db.New(conn)
	now := time.Now().Unix()
	// Add an unavailable peer before reserving the pool.
	if _, err := conn.Exec("INSERT INTO accounts(id,name,provider,account_id,credential,email,plan,enabled,status,created_at,updated_at,models_revision) VALUES('synthetic-unavailable','Synthetic','codex','synthetic-unavailable',x'00','','',1,'ready',1,1,0)"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("INSERT INTO group_accounts(group_id,account_id) VALUES(2,'synthetic-unavailable')"); err != nil {
		t.Fatal(err)
	}
	team, err := s.SaveTeam(ctx, 0, TeamInput{Name: "Synthetic", Enabled: true, MemberIDs: []int64{user}})
	if err != nil {
		t.Fatal(err)
	}
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic", TeamID: team.ID, GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", Period: "upstream", Members: []Share{{UserID: user, Limit: 10000}}, Rates: []Rate{{Model: "synthetic", Input: 1, Cached: 1, Output: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	save := func() {
		row, err := q.GetAccountUsage(ctx, account)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(map[string]any{"updated_at": now, "read_started_at": now * 1000, "limits": []any{map[string]any{"name": "", "windows": []any{map[string]any{"kind": "primary", "used_percent": 0, "reset_at": now + 1000}}}}})
		if _, err = q.SaveAccountUsage(ctx, db.SaveAccountUsageParams{AccountID: account, Snapshot: raw, UpdatedAt: now, Revision: row.Revision}); err != nil {
			t.Fatal(err)
		}
	}
	save()
	if err = s.Refresh(ctx, scheme.ID); err != nil {
		t.Errorf("one missing snapshot blocked healthy account: %v", err)
	}
	if _, err = Check(ctx, q, scheme.ID, user, 2, account, "synthetic", now); err != nil {
		t.Errorf("healthy account: %v", err)
	}
	if _, err = conn.Exec("UPDATE accounts SET models_revision=models_revision+1 WHERE id=?", account); err != nil {
		t.Fatal(err)
	}
	save()
	if _, err = Check(ctx, q, scheme.ID, user, 2, account, "synthetic", now); !errors.Is(err, ErrSnapshot) {
		t.Errorf("changed account revision admitted against old windows: %v", err)
	}

	if err = s.Refresh(ctx, scheme.ID); err != nil {
		t.Fatal("fresh same-identity snapshot did not restore account after lifecycle change", err)
	}
	if _, err = Check(ctx, q, scheme.ID, user, 2, account, "synthetic", now); err != nil {
		t.Fatal(err)
	}
}
