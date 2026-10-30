package resets

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCardPriorityUsesEarliestValidExpiryAndStableTies(t *testing.T) {
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	early := now.Add(time.Hour).Unix()
	late := now.Add(24 * time.Hour).Unix()
	past := now.Add(-time.Hour).Unix()
	cards := []Card{
		{ID: "synthetic-no-expiry", Status: "available", Supported: true, GrantedAt: 1},
		{ID: "synthetic-later", Status: "available", Supported: true, GrantedAt: 2, ExpiresAt: &late},
		{ID: "synthetic-expired", Status: "available", Supported: true, GrantedAt: 1, ExpiresAt: &past},
		{ID: "synthetic-used", Status: "redeemed", Supported: true, GrantedAt: 1, ExpiresAt: &early},
		{ID: "synthetic-unsupported", Status: "available", Supported: false, GrantedAt: 1, ExpiresAt: &early},
		{ID: "synthetic-soon-newer", Status: "available", Supported: true, GrantedAt: 3, ExpiresAt: &early},
		{ID: "synthetic-soon-oldest", Status: "available", Supported: true, GrantedAt: 2, ExpiresAt: &early},
		{ID: "synthetic-redeeming", Status: "redeeming", Supported: true, GrantedAt: 1, ExpiresAt: &early},
	}
	got := PrepareCards(CardList{Cards: cards}, now)
	if got.NextID != "synthetic-soon-oldest" || got.Cards[0].ID != got.NextID {
		t.Fatal(got)
	}
	if cards[0].ID != "synthetic-no-expiry" {
		t.Fatal("priority mutated a shared observation")
	}
	next := PrepareCards(CardList{Cards: cards}, now.Add(2*time.Hour))
	if next.NextID != "synthetic-later" {
		t.Fatal(next)
	}
	last := PrepareCards(CardList{Cards: cards}, now.Add(48*time.Hour))
	if last.NextID != "synthetic-no-expiry" {
		t.Fatal(last)
	}
}

func TestRunPersistsSelectedCardAndRetryNeverSelectsAnother(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	soon := f.now.Add(time.Hour).Unix()
	later := f.now.Add(24 * time.Hour).Unix()
	f.observation.Cards = []Card{{ID: "synthetic-later", Status: "available", Supported: true, ExpiresAt: &later}, {ID: "synthetic-soon", Status: "available", Supported: true, ExpiresAt: &soon}}
	var ids, keys []string
	f.s.redeem = func(_ context.Context, _ int64, _ string, key, card string) (string, error) {
		ids = append(ids, card)
		keys = append(keys, key)
		return "", errors.New("synthetic response lost")
	}
	state, err := f.s.Run(ctx, 1, syntheticAccount, "manual")
	if err != nil || !state.Pending || len(ids) != 1 || ids[0] != "synthetic-soon" {
		t.Fatal(state, ids, err)
	}
	var saved string
	if err := f.db.QueryRow("SELECT credit_id FROM reset_runs WHERE outcome='pending'").Scan(&saved); err != nil || saved != ids[0] {
		t.Fatal(saved, err)
	}
	f.observation.Cards = []Card{{ID: "synthetic-different", Status: "available", Supported: true}}
	other := New(f.db, f.s.location, f.s.inspect, f.s.redeem)
	other.now = f.s.now
	if _, err := other.Retry(ctx, 1, syntheticAccount); err != nil || len(ids) != 2 || ids[0] != ids[1] || keys[0] != keys[1] {
		t.Fatal(ids, keys, err)
	}
	other.redeem = func(_ context.Context, _ int64, _ string, key, card string) (string, error) {
		if key != keys[0] || card != ids[0] {
			t.Fatal("replay identity changed")
		}
		return "already_redeemed", nil
	}
	if state, err := other.Retry(ctx, 1, syntheticAccount); err != nil || state.Pending {
		t.Fatal(state, err)
	}
	if err := f.db.QueryRow("SELECT credit_id FROM reset_runs WHERE request_id=?", keys[0]).Scan(&saved); err != nil || saved != "" {
		t.Fatal("resolved provider card ID was retained", saved, err)
	}
}

func TestLegacyPendingAttemptKeepsItsOmittedCardID(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.db.Exec(`INSERT INTO reset_runs(request_id,account_id,trigger_kind,outcome,reason,started_at,updated_at) VALUES('synthetic-legacy',?,'manual','pending','confirmation_required',1,1)`, syntheticAccount); err != nil {
		t.Fatal(err)
	}
	f.s.redeem = func(_ context.Context, _ int64, _ string, key, card string) (string, error) {
		if key != "synthetic-legacy" || card != "" {
			t.Fatal("legacy attempt acquired a different request body", key, card)
		}
		return "already_redeemed", nil
	}
	if state, err := f.s.Retry(ctx, 1, syntheticAccount); err != nil || state.Pending {
		t.Fatal(state, err)
	}
}

func TestNoEligibleCardDoesNotFallBackToAnUnspecifiedRedemption(t *testing.T) {
	f := setup(t)
	f.observation.Cards = []Card{}
	state, err := f.s.Run(context.Background(), 1, syntheticAccount, "manual")
	if err != nil || f.calls != 0 || len(state.History) != 1 || state.History[0].Reason != "no_eligible_credit" {
		t.Fatal(state, f.calls, err)
	}
}

func TestUnknownCardInventoryNeverFallsBackToProviderSelection(t *testing.T) {
	f := setup(t)
	f.observation.Cards = nil
	state, err := f.s.Run(context.Background(), 1, syntheticAccount, "manual")
	if err != nil || f.calls != 0 || state.History[0].Reason != "cards_unavailable" {
		t.Fatal(state, err)
	}
}

func TestSelectionUsesTimeAfterInventoryRead(t *testing.T) {
	f := setup(t)
	expiry := f.now.Add(time.Second).Unix()
	f.observation.Cards = []Card{{ID: "synthetic-expiring", Status: "available", Supported: true, ExpiresAt: &expiry}}
	f.s.inspect = func(context.Context, int64, string) (Observation, error) {
		f.now = f.now.Add(2 * time.Second)
		return f.observation, nil
	}
	state, err := f.s.Run(context.Background(), 1, syntheticAccount, "manual")
	if err != nil || f.calls != 0 || state.History[0].Reason != "no_eligible_credit" {
		t.Fatal("expired during metadata IO", state, err)
	}
}
