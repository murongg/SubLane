package allocations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage/db"
)

func TestRatioReportSeparatesWaitingFromExceptionsAndAccounts(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	now := time.Now().Unix()
	s.now = func() time.Time { return unix(now) }
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic ratio", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", Period: "upstream", Members: []Share{{UserID: user, Limit: 10000}}, Rates: []Rate{{Model: "synthetic", Input: 1, Output: 1, Cached: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(conn)
	reset := now + 3600
	save := func() {
		raw, _ := json.Marshal(map[string]any{"updated_at": now, "read_started_at": now * 1000, "limits": []any{map[string]any{"name": "", "windows": []any{map[string]any{"kind": "primary", "used_percent": 0, "reset_at": reset}, map[string]any{"kind": "secondary", "used_percent": 0, "reset_at": reset + 86400}}}}})
		row, _ := q.GetAccountUsage(ctx, account)
		if _, err := q.SaveAccountUsage(ctx, db.SaveAccountUsageParams{AccountID: account, Revision: row.Revision, UpdatedAt: now, Snapshot: raw}); err != nil {
			t.Fatal(err)
		}
		if err := s.Refresh(ctx, scheme.ID); err != nil {
			t.Fatal(err)
		}
	}
	save()
	if err := Begin(ctx, q, Request{ID: "synthetic-wait", SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic", StartedAt: now}, now); err != nil {
		t.Fatal(err)
	}
	if err := Finish(ctx, q, "synthetic-wait", Completion{Input: 10, Output: 10, Known: true, Dispatched: true}, now*1000, false); err != nil {
		t.Fatal(err)
	}
	detail, err := s.Detail(ctx, scheme.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Balances) != 2 || detail.Balances[0].Pending != 0 || detail.Balances[0].Syncing != 1 || !detail.Pending[0].Automatic || detail.Balances[0].AccountLabel == "" || detail.Balances[0].AccountID != "" {
		t.Fatal("normal delay presented as manual debt or leaked identity", detail)
	}
	if detail.Balances[0].AccountLabel != detail.Balances[1].AccountLabel {
		t.Fatal("one account has different labels across windows")
	}
	if _, err := Check(ctx, q, scheme.ID, user, 2, account, "synthetic", now); err != nil {
		t.Fatal("ordinary delay blocked request", err)
	}
	now += 121
	save()
	if _, err := Check(ctx, q, scheme.ID, user, 2, account, "synthetic", now); !errors.Is(err, ErrSync) {
		t.Fatal("unbounded delayed usage admitted", err)
	}
	detail, _ = s.Detail(ctx, scheme.ID, user)
	if !detail.Balances[0].SyncPaused || detail.Pending[0].Automatic {
		t.Fatal("prolonged delay has no recovery state", detail)
	}
}

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
	if err := grantPoolMembers(ctx, conn, 2, 3, 4); err != nil {
		t.Fatal(err)
	}
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic ratio", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", Period: "upstream", Members: []Share{{UserID: user, Limit: 5000}, {UserID: 3, Limit: 3000}, {UserID: 4, Limit: 2000}}, Rates: []Rate{{Model: "model-x", Input: 4000000, Cached: 4000000, Output: 4000000}, {Model: "model-y", Input: 1000000, Cached: 1000000, Output: 1000000}}}})
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
	now := int64(1_900_000_000)
	// Scheme activation and admission must share a clock even when CI crosses a second.
	s.now = func() time.Time { return unix(now) }
	// Add an unavailable peer before reserving the pool.
	if _, err := conn.Exec("INSERT INTO accounts(id,name,provider,account_id,credential,email,plan,enabled,status,created_at,updated_at,models_revision) VALUES('synthetic-unavailable','Synthetic','codex','synthetic-unavailable',x'00','','',1,'ready',1,1,0)"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("INSERT INTO group_accounts(group_id,account_id) VALUES(2,'synthetic-unavailable')"); err != nil {
		t.Fatal(err)
	}
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", Period: "upstream", Members: []Share{{UserID: user, Limit: 10000}}, Rates: []Rate{{Model: "synthetic", Input: 1, Cached: 1, Output: 1}}}})
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

func TestRatioUsesBoundedProvisionalWindowBeforePausing(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	q := db.New(conn)
	now := int64(1_900_000_000)
	s.now = func() time.Time { return unix(now) }
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic provisional", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", Period: "upstream", Members: []Share{{UserID: user, Limit: 10000}}, Rates: []Rate{{Model: "synthetic", Input: 1, Cached: 1, Output: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	reset := now + 3600
	raw, _ := json.Marshal(map[string]any{"updated_at": now, "read_started_at": now * 1000, "limits": []any{map[string]any{"name": "", "windows": []any{map[string]any{"kind": "primary", "used_percent": 0, "reset_at": reset}, map[string]any{"kind": "secondary", "used_percent": 0, "reset_at": reset + 86400}}}}})
	row, err := q.GetAccountUsage(ctx, account)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.SaveAccountUsage(ctx, db.SaveAccountUsageParams{AccountID: account, Snapshot: raw, UpdatedAt: now, Revision: row.Revision}); err != nil {
		t.Fatal(err)
	}
	if err = s.Refresh(ctx, scheme.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < ratioProvisionalRequestLimit; i++ {
		id := fmt.Sprintf("synthetic-provisional-%02d", i)
		if err = Begin(ctx, q, Request{ID: id, SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic", StartedAt: now}, now); err != nil {
			t.Fatalf("request %d was paused early: %v", i, err)
		}
		if err = Finish(ctx, q, id, Completion{Known: true, Dispatched: true}, now*1000+int64(i), false); err != nil {
			t.Fatal(err)
		}
	}
	if err = Begin(ctx, q, Request{ID: "synthetic-provisional-over-cap", SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic", StartedAt: now}, now); !errors.Is(err, ErrSync) {
		t.Fatalf("request beyond bounded provisional window was admitted: %v", err)
	}
}

func TestRatioCanBorrowIdleMemberAllowanceWhenEnabled(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	q := db.New(conn)
	other, err := conn.Exec("INSERT INTO users(username,role,password_hash,enabled,created_at) VALUES('synthetic-borrower','member','synthetic',1,1)")
	if err != nil {
		t.Fatal(err)
	}
	otherID, err := other.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if err := grantPoolMembers(ctx, conn, 2, otherID); err != nil {
		t.Fatal(err)
	}
	now := int64(1_900_000_000)
	s.now = func() time.Time { return unix(now) }
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic borrowing", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", Period: "upstream", AllowIdleBorrow: true, Members: []Share{{UserID: user, Limit: 5000}, {UserID: otherID, Limit: 5000}}, Rates: []Rate{{Model: "synthetic", Input: 1, Cached: 1, Output: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	reset := now + 3600
	raw, _ := json.Marshal(map[string]any{"updated_at": now, "read_started_at": now * 1000, "limits": []any{map[string]any{"name": "", "windows": []any{map[string]any{"kind": "primary", "used_percent": 0, "reset_at": reset}, map[string]any{"kind": "secondary", "used_percent": 0, "reset_at": reset + 86400}}}}})
	row, err := q.GetAccountUsage(ctx, account)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.SaveAccountUsage(ctx, db.SaveAccountUsageParams{AccountID: account, Snapshot: raw, UpdatedAt: now, Revision: row.Revision}); err != nil {
		t.Fatal(err)
	}
	if err = s.Refresh(ctx, scheme.ID); err != nil {
		t.Fatal(err)
	}
	if err = Begin(ctx, q, Request{ID: "synthetic-borrowed", SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic", StartedAt: now}, now); err != nil {
		t.Fatal(err)
	}
	if err = Finish(ctx, q, "synthetic-borrowed", Completion{Known: true, Dispatched: true}, now*1000, false); err != nil {
		t.Fatal(err)
	}
	debits, err := q.GetRequestAllocationDebits(ctx, "synthetic-borrowed")
	if err != nil {
		t.Fatal(err)
	}
	points := map[int64]int64{}
	for _, d := range debits {
		if d.Kind == "primary" {
			points[d.WindowID] = 6000
		} else {
			points[d.WindowID] = 0
		}
	}
	if err = s.Settle(ctx, scheme.ID, "synthetic-borrowed", Completion{}, points); err != nil {
		t.Fatal(err)
	}
	// The upstream observation must catch up with the manually confirmed debit;
	// otherwise admission correctly rejects a regressing snapshot.
	raw, _ = json.Marshal(map[string]any{"updated_at": now, "read_started_at": now * 1000, "limits": []any{map[string]any{"name": "", "windows": []any{map[string]any{"kind": "primary", "used_percent": 60, "reset_at": reset}, map[string]any{"kind": "secondary", "used_percent": 0, "reset_at": reset + 86400}}}}})
	row, err = q.GetAccountUsage(ctx, account)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.SaveAccountUsage(ctx, db.SaveAccountUsageParams{AccountID: account, Snapshot: raw, UpdatedAt: now, Revision: row.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err = Check(ctx, q, scheme.ID, user, 2, account, "synthetic", now); err != nil {
		t.Fatalf("idle allowance was not borrowable: %v", err)
	}
	detail, err := s.Detail(ctx, scheme.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, balance := range detail.Balances {
		if balance.UserID == user && balance.Borrowed == 1000 {
			found = true
		}
	}
	if !found {
		t.Fatalf("borrowed points were not reported: %+v", detail.Balances)
	}
}
