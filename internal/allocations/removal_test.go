package allocations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/tenants"
)

func TestRemovedMemberKeepsAllowanceHistoryAndInFlightSettlement(t *testing.T) {
	s, conn, user, account := fixture(t)
	ctx := context.Background()
	now := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic allowance", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(conn)
	r := Request{ID: "synthetic-removal", SchemeID: scheme.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic", StartedAt: now.Unix()}
	if err := Begin(ctx, q, r, now.Unix(), time.UTC); err != nil {
		t.Fatal(err)
	}
	tenancy := tenants.New(conn)
	if err := tenancy.RemoveMember(ctx, 1, 1, user); err != nil {
		t.Fatal(err)
	}
	if _, err := Authorize(ctx, q, scheme.ID, user, 2, now.Unix()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("removed member authorized: %v", err)
	}
	if err := Finish(ctx, q, r.ID, Completion{Input: 42, Known: true, Dispatched: true}, now.UnixMilli(), false); err != nil {
		t.Fatalf("removal interrupted settlement: %v", err)
	}
	detail, err := s.Detail(ctx, scheme.ID, 0)
	if err != nil || len(detail.Balances) != 1 || detail.Balances[0].Used != 42 {
		t.Fatalf("history lost: %+v %v", detail, err)
	}
	if !detail.Balances[0].Removed {
		t.Fatal("historical member not marked as removed")
	}
	if _, err := tenancy.AddMemberByUsername(ctx, 1, 1, "synthetic-member", tenants.RoleMember); err != nil {
		t.Fatal(err)
	}
	if _, err := Authorize(ctx, q, scheme.ID, user, 2, now.Unix()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("rejoin restored resource grant: %v", err)
	}
	if err := groups.New(conn).SetMemberGroups(ctx, user, []int64{2}); err != nil {
		t.Fatal(err)
	}
	detail, err = s.Detail(ctx, scheme.ID, 0)
	if err != nil || detail.Balances[0].Used != 42 || detail.Balances[0].Removed {
		t.Fatalf("rejoin reset allowance history: %+v %v", detail, err)
	}
}
