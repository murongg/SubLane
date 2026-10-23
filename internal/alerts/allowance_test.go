package alerts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAllowanceAlertsDeduplicateAcrossRestartsAndRearmNextCycle(t *testing.T) {
	ctx := context.Background()
	var events []Event
	s, account := fixture(t, senderFunc(func(r *http.Request) (*http.Response, error) {
		var event Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	if _, err := s.conn.Exec("UPDATE accounts SET status='ready' WHERE id=?", account); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, 1, Input{Enabled: true, URL: "https://hooks.example.test/allowance"}); err != nil {
		t.Fatal(err)
	}
	cycle := "1:1900086400"
	failed := false
	observer := func(context.Context, int64) ([]Allowance, error) {
		if failed {
			return nil, ErrCapacityUnavailable
		}
		return []Allowance{{Subject: "1:2:0:80", Cycle: cycle}}, nil
	}
	s.SetAllowanceObserver(observer)
	for range 2 {
		if err := s.Check(ctx); err != nil {
			t.Fatal(err)
		}
	}
	restarted := New(s.conn, s.vault, s.sender)
	restarted.now = s.now
	restarted.SetAllowanceObserver(observer)
	if err := restarted.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Kind != "allowance_low" {
		t.Fatal(events)
	}
	restarted.SetAllowanceObserver(nil)
	if err := restarted.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatal("missing observer claimed recovery", events)
	}
	restarted.SetAllowanceObserver(observer)
	failed = true
	if err := restarted.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatal("observer failure claimed recovery", events)
	}
	failed = false
	cycle = "1:1900172800"
	if err := restarted.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Status != "firing" || events[1].ID == events[0].ID {
		t.Fatal("new period did not rearm", events)
	}
	var count int
	if err := s.conn.QueryRow("SELECT COUNT(*) FROM alert_states WHERE kind='allowance_low'").Scan(&count); err != nil || count != 1 {
		t.Fatal("periods accumulated state", count, err)
	}
}
