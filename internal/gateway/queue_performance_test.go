package gateway

import (
	"context"
	"github.com/murongg/SubLane/internal/groups"
	"strings"
	"testing"
	"time"
)

func TestCapacityReleaseDoesNotRescanUnrelatedPools(t *testing.T) {
	s, meter := measuredGateway(t, 2)
	ctx := context.Background()
	rows, err := s.accounts.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pools := groups.New(s.db)
	if _, err := pools.Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{rows[0].ID}}); err != nil {
		t.Fatal(err)
	}
	other, err := pools.Save(ctx, 0, groups.Input{Name: "Synthetic other", Enabled: true, AccountIDs: []string{rows[1].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetConcurrency(ctx, rows[0].ID, 1); err != nil {
		t.Fatal(err)
	}
	first := receiveQueued(t, queuedOpen(s, ctx, nil))
	if first.err != nil {
		t.Fatal(first.err)
	}
	defer first.exchange.Body.Close()
	unrelated, err := s.Open(ctx, 1, other.ID, []byte(`{"model":"synthetic-model","input":"synthetic"}`), nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	defer unrelated.Body.Close()
	waitCtx, cancel := context.WithCancel(ctx)
	pending := queuedOpen(s, waitCtx, nil)
	defer func() {
		cancel()
		result := receiveQueued(t, pending)
		if result.exchange != nil {
			result.exchange.Body.Close()
		}
	}()
	waitForQueue(t, s, 1)
	meter.take()
	unrelated.Body.Close()
	// Observe notifications before the independent 250ms policy recheck is due.
	time.Sleep(50 * time.Millisecond)
	for _, query := range meter.take() {
		if strings.Contains(query.sql, "-- name: ListAccountRuntime") {
			t.Fatal("an unrelated pool's release restarted waiting selection")
		}
	}
}
