package gateway

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage/db"
)

func TestActivityUsesUTCHoursAndSessionOwnershipAcrossWeeks(t *testing.T) {
	ctx := context.Background()
	s, _ := providerGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected upstream"); return nil, nil }))
	member := runtimeMember(t, s)
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	since := now.AddDate(0, 0, -7).Add(-90 * time.Minute).Unix()
	if _, err := s.db.Exec("UPDATE usage_hourly_coverage SET started_at=?", since); err != nil {
		t.Fatal(err)
	}
	input, ownerInput := int64(10), int64(70)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, r := range []db.RecordRequestParams{
		{UserID: member, GroupID: 1, StartedAt: now.AddDate(0, 0, -7).Add(-45 * time.Minute).Unix(), Outcome: "success", InputTokens: &input},
		{UserID: member, GroupID: 1, StartedAt: now.Add(-50 * time.Minute).Unix(), Outcome: "success"},
		{UserID: 1, GroupID: 1, StartedAt: now.AddDate(0, 0, -7).Add(-40 * time.Minute).Unix(), Outcome: "success", InputTokens: &ownerInput},
	} {
		if err := recordStatistics(ctx, s.queries.WithTx(tx), r); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	mine, err := s.UserStatistics(ctx, member, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine.Activity.Cells) != 168 || mine.Activity.TrackingSince != since {
		t.Fatal("unbounded or missing hourly coverage")
	}
	cell := mine.Activity.Cells[9]
	if cell.Weekday != 0 || cell.Hour != 9 || cell.Requests != 2 || cell.InputTokens != 10 || cell.InputReported != 1 || cell.Samples != 2 {
		t.Fatal("wrong owner/hour/week aggregation", cell)
	}
	all, err := s.Statistics(ctx, 30)
	if err != nil || all.Activity.Cells[9].Requests != 3 || all.Activity.Cells[9].InputTokens != 80 {
		t.Fatal("incorrect team aggregation", err)
	}
	today, err := s.UserStatistics(ctx, member, 1)
	if err != nil || today.Activity.Cells[9].Requests != 1 || today.Activity.Cells[9].InputReported != 0 {
		t.Fatal("period filter or missing-token semantics", err)
	}
	if today.Activity.Cells[0].Samples != 1 || today.Activity.Cells[0].Requests != 0 || today.Activity.Cells[10].Samples != 1 || today.Activity.Cells[11].Samples != 0 || today.Activity.Cells[24].Samples != 0 {
		t.Fatal("zero activity and future hours were conflated")
	}
	if _, err := s.db.Exec("UPDATE usage_hourly_coverage SET started_at=?", now.Unix()); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.UserStatistics(ctx, member, 1)
	if err != nil || fresh.Activity.Cells[9].Samples != 0 || fresh.Activity.Cells[9].Requests != 0 || fresh.Activity.Cells[10].Samples != 1 {
		t.Fatal("pre-collection hours presented as collected", err)
	}
}

func TestHourlyWriteFailureRollsBackHistoryAndDailyCounters(t *testing.T) {
	ctx := context.Background()
	s, _ := providerGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	if _, err := s.db.Exec("CREATE TRIGGER fail_hourly BEFORE INSERT ON usage_hourly BEGIN SELECT RAISE(ABORT,'synthetic failure'); END"); err != nil {
		t.Fatal(err)
	}
	r, err := s.Open(ctx, 1, 1, []byte(`{"model":"synthetic-model","input":"synthetic"}`), nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Events(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	history, err := s.Requests(ctx, 0, "", "")
	if err != nil || len(history.Requests) != 0 {
		t.Fatal("history committed without hourly usage", err)
	}
	page, err := s.Statistics(ctx, 7)
	if err != nil || page.Totals.Requests != 0 {
		t.Fatal("daily usage committed without hourly usage", err)
	}
}
