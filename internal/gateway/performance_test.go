package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/upstream"
	"net/http"
	"testing"
	"time"
)

func TestFreshPoolSnapshotsUseBatchedReads(t *testing.T) {
	s, meter := measuredGateway(t, 20)
	ctx := context.Background()
	for _, test := range []struct {
		name   string
		budget int
		read   func() error
	}{
		{"catalog", 10, func() error { return s.warmCatalogs(ctx, 1, 1, "", true) }},
		{"quota", 4, func() error { return s.warmUsage(ctx, 1, 1, "") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			meter.take()
			if err := test.read(); err != nil {
				t.Fatal(err)
			}
			if count := len(meter.take()); count > test.budget {
				t.Fatalf("20 fresh accounts required %d SQL calls; budget is %d", count, test.budget)
			}
		})
	}
}

func TestSelectionReadsPoolSnapshotsInBatches(t *testing.T) {
	s, meter := measuredGateway(t, 20)
	ctx := context.Background()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, _, err := s.selectAccount(ctx, 1, 1, "", "", "synthetic-model", Responses); err != nil {
		t.Fatal(err)
	}
	if count := len(meter.take()); count > 35 {
		t.Errorf("selection required %d SQL calls for 20 accounts; budget is 35", count)
	}
	for _, state := range s.health {
		state.InFlight = state.MaxConcurrency
	}
	if _, _, err := s.selectAccount(ctx, 1, 1, "", "", "synthetic-model", Responses); !errors.Is(err, ErrAccountBusy) {
		t.Fatal("saturated pool was not busy", err)
	}
	if count := len(meter.take()); count > 12 {
		t.Errorf("saturated selection required %d SQL calls; budget is 12", count)
	}
}

func TestFreshLargeQuotaWarmupReusesDecodedSnapshots(t *testing.T) {
	s, ids := providerGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("fresh quota triggered provider IO")
		return nil, nil
	}))
	ctx := context.Background()
	if _, err := s.db.Exec("DELETE FROM accounts WHERE provider!='antigravity'"); err != nil {
		t.Fatal(err)
	}
	value := upstream.Usage{UpdatedAt: time.Now().Unix(), Limits: []upstream.UsageLimit{}}
	for i := range 512 {
		value.Limits = append(value.Limits, upstream.UsageLimit{Name: fmt.Sprintf("synthetic-model-%d", i), Windows: []upstream.UsageWindow{}})
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.queries.SaveAccountUsage(ctx, db.SaveAccountUsageParams{AccountID: ids["antigravity"], Snapshot: raw, UpdatedAt: value.UpdatedAt, Revision: 0}); err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(5, func() {
		if err := s.warmUsage(ctx, 1, 1, ""); err != nil {
			t.Fatal(err)
		}
	})
	t.Logf("fresh 512-model quota warmup: %.0f allocations", allocations)
	if allocations > 200 {
		t.Fatalf("fresh 512-model quota required %.0f allocations per warmup; decoded snapshots should be reused", allocations)
	}
}
