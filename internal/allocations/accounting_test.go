package allocations

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage/db"
)

func busyAllowance(t *testing.T) (*Service, Scheme, int64, string, *int64) {
	t.Helper()
	s, conn, user, account := fixture(t)
	now := syntheticClock(s)
	ctx := context.Background()
	ids := []int64{1, user}
	for len(ids) < 10 {
		result, err := conn.Exec(`INSERT INTO users(username,password_hash,role,created_at) VALUES(?,'synthetic-hash','member',1)`, fmt.Sprintf("synthetic-busy-%d", len(ids)))
		if err != nil {
			t.Fatal(err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		if err := grantPoolMembers(ctx, conn, 2, id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	config := Config{Mode: "ratio", RatioUnit: "amount", Period: "durations"}
	for _, id := range ids {
		config.Members = append(config.Members, Share{UserID: id, Limit: 1000})
	}
	for i := 1; i <= 8; i++ {
		config.Windows = append(config.Windows, WindowCondition{DurationSeconds: int64(i * 3600), Limit: 1_000_000})
	}
	for i := 0; i < 4096; i++ {
		config.Rates = append(config.Rates, Rate{Model: fmt.Sprintf("synthetic-%04d", i), Input: 1_000_000, Cached: 1_000_000, Output: 1_000_000})
	}
	out, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic busy allowance", GroupID: 2, Enabled: true, Config: config})
	if err != nil {
		t.Fatal(err)
	}
	start, end := effectiveWindow(out.Revision, *now, time.UTC)
	q := db.New(conn)
	for _, id := range ids {
		for i := 0; i < 4; i++ {
			err := q.BeginAllocationEntry(ctx, db.BeginAllocationEntryParams{RequestID: fmt.Sprintf("synthetic-busy-%d-%d", id, i), SchemeID: out.ID, RevisionID: out.Revision.ID, UserID: id, AccountID: account, Model: "synthetic-0000", Mode: "amount", WindowStart: start, ResetAt: end, StartedAt: *now})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	// Keep active requests on a historical revision, with their original limits and prices.
	in := SchemeInput{Name: out.Name, GroupID: 2, Enabled: true, ApplyNow: true, Config: out.Config}
	in.Config.Members = append([]Share(nil), out.Config.Members...)
	in.Config.Members[0].Limit = 900
	out, err = s.SaveScheme(ctx, out.ID, conditioned(t, s, out.ID, in))
	if err != nil {
		t.Fatal(err)
	}
	return s, out, user, account, now
}

func TestBusyAllowanceAccountingStaysBounded(t *testing.T) {
	s, out, user, account, now := busyAllowance(t)
	ctx := context.Background()
	t.Run("report", func(t *testing.T) {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		started := time.Now()
		detail, err := s.Detail(ctx, out.ID, 0)
		runtime.ReadMemStats(&after)
		allocated := after.TotalAlloc - before.TotalAlloc
		t.Logf("report: %v, %.1f MiB allocated", time.Since(started), float64(allocated)/(1<<20))
		if err != nil || len(detail.Balances) != 80 {
			t.Fatal(len(detail.Balances), err)
		}
		for _, balance := range detail.Balances {
			if balance.Used != 0 || balance.Reserved != 40_000 || balance.InFlight != 4 || balance.Admission != "risk_limited" {
				t.Fatal("large price catalogs changed captured exposure", balance)
			}
		}
		// Leave ample room for the catalog itself, but not a copy for every member/window/request.
		if allocated > 64<<20 {
			t.Errorf("one report allocated %d bytes for a bounded allowance", allocated)
		}
	})
	t.Run("admission", func(t *testing.T) {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		_, err := Check(ctx, db.New(s.conn), out.ID, user, 2, account, "synthetic-0000", *now, time.UTC)
		runtime.ReadMemStats(&after)
		if err != ErrRisk {
			t.Fatal("ongoing requests stopped limiting admission", err)
		}
		if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 16<<20 {
			t.Errorf("one admission allocated %d bytes for a bounded allowance", allocated)
		}
	})
	if err := Finish(ctx, db.New(s.conn), "synthetic-busy-1-0", Completion{Known: true, Dispatched: true, Input: 30}, *now*1000, false); err != nil {
		t.Fatal(err)
	}
	detail, err := s.Detail(ctx, out.ID, 1)
	if err != nil || len(detail.Balances) != 8 {
		t.Fatal(detail, err)
	}
	for _, balance := range detail.Balances {
		if balance.InFlight != 3 || balance.Reserved != 30_000 || balance.Used != 30 {
			t.Fatal("an earlier read retained stale reservations after settlement", balance)
		}
	}
}
