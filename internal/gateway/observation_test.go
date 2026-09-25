package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/upstream"
)

func TestResponseQuotaPreservesAccountingBoundaryAndLifecycle(t *testing.T) {
	f := newQuotaFixture(t)
	first := waitQuota(t, f)
	ctx := context.Background()
	row, err := f.service.queries.GetAccountUsage(ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	value := upstream.Usage{}
	raw, _ := json.Marshal(first.Usage)
	json.Unmarshal(raw, &value)
	value.UpdatedAt += 6
	value.ReadStartedAt += 1000
	points := 30.0
	value.Limits[0].Windows[0].UsedPercent = &points
	f.clock.Add(6)
	if err := f.service.observeUsage(ctx, f.id, row.Revision, value); err != nil {
		t.Fatal(err)
	}
	saved, _ := f.service.queries.GetAccountUsage(ctx, f.id)
	got := savedUsage(saved)
	if got.ReadStartedAt != value.ReadStartedAt || *got.Limits[0].Windows[0].UsedPercent != 30 {
		t.Fatal("lost observation boundary", got)
	}
	visible, err := f.service.Usage(ctx, f.id)
	if err != nil || *visible.Limits[0].Windows[0].UsedPercent != 30 || f.calls.Load() != 1 {
		t.Fatal("cache not updated", visible, err)
	}
	// A late response must not overwrite a newer baseline or pretend a partial window is complete.
	value.ReadStartedAt = first.ReadStartedAt
	points = 40
	f.service.observeUsage(ctx, f.id, row.Revision, value)
	value.ReadStartedAt += 9000
	value.Limits[0].Windows = nil
	f.service.observeUsage(ctx, f.id, row.Revision, value)
	saved, _ = f.service.queries.GetAccountUsage(ctx, f.id)
	if *savedUsage(saved).Limits[0].Windows[0].UsedPercent != 30 {
		t.Fatal("late or incomplete response overwrote baseline")
	}
	if _, err := f.connection.Exec("UPDATE accounts SET models_revision=models_revision+1 WHERE id=?", f.id); err != nil {
		t.Fatal(err)
	}
	if err := f.service.observeUsage(ctx, f.id, row.Revision, first.Usage); err != nil {
		t.Fatal(err)
	}
	saved, _ = f.service.queries.GetAccountUsage(ctx, f.id)
	if savedUsage(saved) != nil {
		t.Fatal("old lifecycle observation restored")
	}
}

func TestQuotaObservationUsesDispatchBoundaryAfterPreflight(t *testing.T) {
	for _, event := range []bool{false, true} {
		t.Run(map[bool]string{false: "headers", true: "event"}[event], func(t *testing.T) {
			s, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
				response := syntheticStream()
				if event {
					original, _ := io.ReadAll(response.Body)
					response.Body = io.NopCloser(strings.NewReader("data: {\"type\":\"codex.rate_limits\",\"rate_limits\":{\"primary\":{\"used_percent\":30,\"window_minutes\":300,\"reset_at\":2000000000}}}\n\n" + string(original)))
				} else {
					response.Header.Set("X-Codex-Primary-Used-Percent", "30")
					response.Header.Set("X-Codex-Primary-Reset-At", "2000000000")
					response.Header.Set("X-Codex-Primary-Window-Minutes", "300")
				}
				return response, nil
			}))
			now := time.Now().Truncate(time.Second)
			var reads atomic.Int64
			s.now = func() time.Time {
				if reads.Add(1) == 1 {
					return now
				}
				return now.Add(time.Second)
			}
			s.usage.now = func() time.Time { return now.Add(time.Second) }
			raw, _ := json.Marshal(map[string]any{"updated_at": now.Unix(), "read_started_at": now.UnixMilli() + 500, "limits": []any{map[string]any{"name": "", "windows": []any{map[string]any{"kind": "primary", "used_percent": 25, "reset_at": int64(2000000000)}}}}})
			if _, err := s.db.Exec(`INSERT INTO account_usage(account_id,snapshot,updated_at,revision) SELECT id,?, ?,models_revision FROM accounts WHERE id=?`, raw, now.Unix(), ids["codex"]); err != nil {
				t.Fatal(err)
			}
			x, err := s.Open(context.Background(), 1, 1, []byte(`{"model":"synthetic-model","input":[]}`), nil, Responses)
			if err != nil {
				t.Fatal(err)
			}
			if err := x.Events(func([]byte) error { return nil }); err != nil {
				t.Fatal(err)
			}
			x.Body.Close()
			row, err := s.queries.GetAccountUsage(context.Background(), ids["codex"])
			if err != nil {
				t.Fatal(err)
			}
			got := savedUsage(row)
			if got == nil || *got.Limits[0].Windows[0].UsedPercent != 30 || got.ReadStartedAt != now.Add(time.Second).UnixMilli() {
				t.Fatalf("response observation lost after preflight: %+v", got)
			}
		})
	}
}

func TestOlderPollCannotReplaceResponseObservation(t *testing.T) {
	f := newQuotaFixture(t)
	baseline := waitQuota(t, f)
	f.clock.Add(6)
	f.block, f.started = make(chan struct{}), make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() { _, err := f.service.RefreshUsage(context.Background(), f.id); done <- err }()
	select {
	case <-f.started:
	case <-time.After(time.Second):
		t.Fatal("poll did not start")
	}
	f.clock.Add(1)
	row, err := f.service.queries.GetAccountUsage(context.Background(), f.id)
	if err != nil {
		close(f.block)
		t.Fatal(err)
	}
	newer := baseline.Usage
	points := 30.0
	newer.Limits[0].Windows[0].UsedPercent = &points
	newer.ReadStartedAt = f.clock.Load() * 1000
	newer.UpdatedAt = f.clock.Load()
	if err := f.service.observeUsage(context.Background(), f.id, row.Revision, newer); err != nil {
		close(f.block)
		t.Fatal(err)
	}
	close(f.block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	saved, err := f.service.queries.GetAccountUsage(context.Background(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	got := savedUsage(saved)
	if got.ReadStartedAt != newer.ReadStartedAt || *got.Limits[0].Windows[0].UsedPercent != 30 {
		t.Fatal("older in-flight poll overwrote response observation")
	}
}
