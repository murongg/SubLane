package allocations

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage/db"
)

func TestExistingEditsRequireVersionConditions(t *testing.T) {
	s, _, user, _ := fixture(t)
	ctx := context.Background()
	in := SchemeInput{Name: "Synthetic conditioned budget", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}}
	scheme, err := s.SaveScheme(ctx, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveScheme(ctx, scheme.ID, in)
	if err == nil || !strings.Contains(err.Error(), "allocation_version_required") {
		t.Fatal("unconditional write accepted", err)
	}
}

func TestSchemeInputKeepsStrictUnknownFieldValidation(t *testing.T) {
	var in SchemeInput
	if err := json.Unmarshal([]byte(`{"name":"Synthetic","accounting_anchor_at":1}`), &in); err == nil {
		t.Fatal("read-only anchor accepted in mutation payload")
	}
}

func TestImmediateMemberEditAcceptsEquivalentLegacyClockDefaults(t *testing.T) {
	s, _, user, _ := fixture(t)
	syntheticClock(s)
	ctx := context.Background()
	in := SchemeInput{Name: "Synthetic default clock", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "month", Members: []Share{{UserID: user, Limit: 100}}}}
	out, err := s.SaveScheme(ctx, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	in.Config.ResetTime = "00:00"
	in.Config.ResetDay = 1
	in.Config.Members = []Share{{UserID: user, Limit: 90}}
	in.ApplyNow = true
	_, err = s.SaveScheme(ctx, out.ID, conditioned(t, s, out.ID, in))
	if err != nil {
		t.Fatal("equivalent defaults rejected", err)
	}
}

func TestRevisionConditionsDoNotUseTimestampBounds(t *testing.T) {
	id := int64(999_999_999_999)
	raw, _ := json.Marshal(id)
	err := checkVersions(Conditions{ExpectedCurrentRevisionID: raw, ExpectedNextRevisionID: json.RawMessage("null")}, Scheme{WriteConditions: WriteConditions{Current: &id}})
	if err != nil {
		t.Fatal("valid persisted revision ID treated as a time", err)
	}
}

func TestImmediateSharesRejectUnfundedThirdRecipientAndKeepConsumption(t *testing.T) {
	s, conn, user, account := fixture(t)
	now := syntheticClock(s)
	ctx := context.Background()
	identity, err := auth.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	third, err := identity.CreateMember(ctx, "synthetic-third", "synthetic-pass")
	if err != nil {
		t.Fatal(err)
	}
	if err := grantPoolMembers(ctx, conn, 2, third.ID); err != nil {
		t.Fatal(err)
	}
	in := SchemeInput{Name: "Synthetic remaining budget", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", RatioUnit: "tokens", Total: 100, Period: "day", Members: []Share{{UserID: 1, Limit: 5000}, {UserID: user, Limit: 5000}}}}
	out, err := s.SaveScheme(ctx, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(conn)
	r := Request{ID: "synthetic-used-forty", SchemeID: out.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic", StartedAt: *now}
	if err := Begin(ctx, q, r, *now, time.UTC); err != nil {
		t.Fatal(err)
	}
	if err := Finish(ctx, q, r.ID, Completion{Input: 40, Known: true, Dispatched: true}, *now*1000, false); err != nil {
		t.Fatal(err)
	}
	in.ApplyNow = true
	in.Config.Members = []Share{{UserID: 1, Limit: 3333}, {UserID: user, Limit: 3333}, {UserID: third.ID, Limit: 3333}}
	if _, err := s.SaveScheme(ctx, out.ID, conditioned(t, s, out.ID, in)); err != ErrFunding {
		t.Fatal("unfunded shares accepted", err)
	}
	in.Config.Members = []Share{{UserID: 1, Limit: 3000}, {UserID: user, Limit: 4000}, {UserID: third.ID, Limit: 3000}}
	if _, err := s.SaveScheme(ctx, out.ID, conditioned(t, s, out.ID, in)); err != nil {
		t.Fatal("funded redistribution failed", err)
	}
	detail, err := s.Detail(ctx, out.ID, user)
	if err != nil || detail.Balances[0].Used != 40 || detail.Balances[0].Limit != 40 {
		t.Fatal("reallocation cleared usage", detail.Balances, err)
	}
}

func syntheticClock(s *Service) *int64 {
	now := int64(1_900_000_000)
	s.now = func() time.Time { return time.Unix(now, 0) }
	return &now
}

func conditioned(t *testing.T, s *Service, id int64, in SchemeInput) SchemeInput {
	t.Helper()
	ctx := context.Background()
	state, err := s.readSchemeCore(ctx, db.New(s.conn), id, s.now().Unix())
	if err != nil {
		return in
	}
	data, _ := json.Marshal(in)
	var body map[string]any
	json.Unmarshal(data, &body)
	current, err := Current(ctx, db.New(s.conn), id, s.now().Unix())
	body["expected_current_revision_id"] = nil
	if err == nil {
		body["expected_current_revision_id"] = current.ID
	}
	body["expected_next_revision_id"] = nil
	if state.Next != nil {
		body["expected_next_revision_id"] = state.Next.ID
	}
	body["expected_effective_at"] = state.EditEffectiveAt
	base := state.Revision
	if state.Next != nil {
		base = *state.Next
	}
	body["confirm_replacement"] = !sameSchedule(base.Config, in.Config)
	data, _ = json.Marshal(body)
	json.Unmarshal(data, &in)
	return in
}

func TestCreationOptionalTimeDoesNotSlideOrFallback(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"null", nil, "allocation_effective_at_required"},
		{"wrong-type", "1900018000", "invalid_allocation_input"},
		{"expired", int64(1_900_000_000), "allocation_effective_at_conflict"},
		{"confirmed", int64(1_900_018_000), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, conn, user, _ := fixture(t)
			now := syntheticClock(s)
			*now += 60
			in := SchemeInput{Name: "Synthetic optional time", GroupID: 2, Enabled: true, StartNext: true, Config: Config{Mode: "windows", Period: "durations", Windows: []WindowCondition{{DurationSeconds: 18000, Limit: 100}}, Members: []Share{{UserID: user}}, Rates: []Rate{{Model: "synthetic", Input: 1, Output: 1}}}}
			data, _ := json.Marshal(in)
			var body map[string]any
			json.Unmarshal(data, &body)
			body["expected_effective_at"] = tc.value
			data, _ = json.Marshal(body)
			json.Unmarshal(data, &in)
			out, err := s.SaveScheme(context.Background(), 0, in)
			if tc.want != "" {
				if err == nil || err.Error() != tc.want {
					t.Fatal("wrong failure", out, err)
				}
				var count int
				conn.QueryRow(`SELECT count(*) FROM allocation_schemes`).Scan(&count)
				if count != 0 {
					t.Fatal("failed creation wrote data", count)
				}
				return
			}
			if err != nil || out.EffectiveAt != tc.value {
				t.Fatal("confirmed start moved", out, err)
			}
		})
	}
}

func TestNaturalBoundaryExpiresConfirmationWithoutVersionChange(t *testing.T) {
	s, conn, user, _ := fixture(t)
	now := syntheticClock(s)
	start := *now
	in := SchemeInput{Name: "Synthetic time fence", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}}
	out, err := s.SaveScheme(context.Background(), 0, in)
	if err != nil {
		t.Fatal(err)
	}
	in = conditioned(t, s, out.ID, in)
	boundary := out.EditEffectiveAt
	*now = boundary + 1
	_, err = s.SaveScheme(context.Background(), out.ID, in)
	if err == nil || err.Error() != "allocation_effective_at_conflict" {
		t.Fatal("expired confirmation accepted", err)
	}
	var count int
	conn.QueryRow(`SELECT count(*) FROM allocation_revisions`).Scan(&count)
	if count != 1 {
		t.Fatal("time conflict wrote a revision", count, start)
	}
}

func TestImmediateMembersPreserveAnchorNextAndStaleWriteProtection(t *testing.T) {
	s, _, user, _ := fixture(t)
	ctx := context.Background()
	now := syntheticClock(s)
	in := SchemeInput{Name: "Synthetic safe members", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", RatioUnit: "tokens", Total: 100, Period: "day", Members: []Share{{UserID: 1, Limit: 5000}, {UserID: user, Limit: 5000}}}}
	created, err := s.SaveScheme(ctx, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	in.Config.Total = 120
	planned, err := s.SaveScheme(ctx, created.ID, conditioned(t, s, created.ID, in))
	if err != nil || planned.Next == nil {
		t.Fatal(planned, err)
	}
	stale := conditioned(t, s, created.ID, in)
	in.Config = created.Config
	in.ApplyNow = true
	in.Config.Members = []Share{{UserID: 1, Limit: 6000}, {UserID: user, Limit: 4000}}
	updated, err := s.SaveScheme(ctx, created.ID, conditioned(t, s, created.ID, in))
	if err != nil || updated.Next == nil || updated.AccountingAnchorAt != *now || updated.Next.EffectiveAt != planned.Next.EffectiveAt || updated.Next.Config.Total != 120 || updated.Next.Config.Members[0].Limit != 6000 {
		t.Fatal("member edit lost schedule", updated, err)
	}
	if _, err := s.SaveScheme(ctx, created.ID, stale); err == nil || err.Error() != "allocation_version_conflict" {
		t.Fatal("stale full write accepted", err)
	}
	in.Config = updated.Config
	in.Config.Members = []Share{{UserID: 1, Limit: 5500}, {UserID: user, Limit: 4500}}
	second, err := s.SaveScheme(ctx, created.ID, conditioned(t, s, created.ID, in))
	if err != nil || second.Revision.ID <= updated.Revision.ID {
		t.Fatal("same-second immediate save failed", second, err)
	}
}

func TestIndependentFutureMembersRejectProjection(t *testing.T) {
	s, conn, user, _ := fixture(t)
	ctx := context.Background()
	now := syntheticClock(s)
	in := SchemeInput{Name: "Synthetic independent next", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", RatioUnit: "tokens", Total: 100, Period: "day", Members: []Share{{UserID: 1, Limit: 5000}, {UserID: user, Limit: 5000}}}}
	out, err := s.SaveScheme(ctx, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	future := out.Config
	future.Members = []Share{{UserID: 1, Limit: 3000}, {UserID: user, Limit: 7000}}
	raw, _ := json.Marshal(future)
	if _, err := db.New(conn).SaveAllocationRevision(ctx, db.SaveAllocationRevisionParams{SchemeID: out.ID, EffectiveAt: *now + 3600, AccountingAnchorAt: *now + 3600, Config: string(raw)}); err != nil {
		t.Fatal(err)
	}
	in.ApplyNow = true
	in.Config.Members = []Share{{UserID: 1, Limit: 6000}, {UserID: user, Limit: 4000}}
	_, err = s.SaveScheme(ctx, out.ID, conditioned(t, s, out.ID, in))
	if err == nil || err.Error() != "allocation_member_conflict" {
		t.Fatal("independent plan overwritten", err)
	}
}

func TestMemberRemovalKeepsCapturedReservationUntilSettlement(t *testing.T) {
	s, conn, user, account := fixture(t)
	now := syntheticClock(s)
	ctx := context.Background()
	in := SchemeInput{Name: "Synthetic captured reservation", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", RatioUnit: "tokens", Total: 100, Period: "day", Members: []Share{{UserID: 1, Limit: 5000}, {UserID: user, Limit: 5000}}}}
	out, err := s.SaveScheme(ctx, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(conn)
	r := Request{ID: "synthetic-captured-hold", SchemeID: out.ID, UserID: user, GroupID: 2, AccountID: account, Model: "synthetic", StartedAt: *now}
	if err := Begin(ctx, q, r, *now, time.UTC); err != nil {
		t.Fatal(err)
	}
	in.ApplyNow = true
	in.Config.Members = []Share{{UserID: 1, Limit: 5000}}
	out, err = s.SaveScheme(ctx, out.ID, conditioned(t, s, out.ID, in))
	if err != nil {
		t.Fatal("revocation failed", err)
	}
	window := effectiveWindows(out.Revision, *now, time.UTC)[0]
	_, held, _, err := groupRoom(ctx, q, out.ID, out.Config, window)
	if err != nil || held != 5 {
		t.Fatal("revocation resized or released the admitted reservation", held, err)
	}
	in.Config.Members[0].Limit = 10000
	if _, err := s.SaveScheme(ctx, out.ID, conditioned(t, s, out.ID, in)); err != ErrFunding {
		t.Fatal("an ongoing former member's reservation funded another recipient", err)
	}
	if err := Finish(ctx, q, r.ID, Completion{Input: 40, Known: true, Dispatched: true}, *now*1000, false); err != nil {
		t.Fatal(err)
	}
	in.Config.Members[0].Limit = 6000
	if _, err := s.SaveScheme(ctx, out.ID, conditioned(t, s, out.ID, in)); err != nil {
		t.Fatal("settled usage plus remaining entitlement did not fit", err)
	}
}

func TestBudgetEditPreservesRecipientsAfterIndependentAccessRevocation(t *testing.T) {
	s, conn, user, _ := fixture(t)
	now := syntheticClock(s)
	ctx := context.Background()
	in := SchemeInput{Name: "Synthetic retained recipients", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", RatioUnit: "tokens", Total: 100, Period: "day", Members: []Share{{UserID: 1, Limit: 5000}, {UserID: user, Limit: 5000}}}}
	out, err := s.SaveScheme(ctx, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := groups.New(conn).SetMemberGroups(ctx, user, []int64{}); err != nil {
		t.Fatal(err)
	}
	in.Config.Total = 120
	updated, err := s.SaveScheme(ctx, out.ID, conditioned(t, s, out.ID, in))
	if err != nil || updated.Next == nil || !sameMembers(updated.Next.Config.Members, out.Config.Members) {
		t.Fatal("budget edit lost or rejected unchanged historical recipients", updated, err)
	}
	*now = updated.Next.EffectiveAt
	if _, err := Authorize(ctx, db.New(conn), out.ID, user, 2, *now); err != ErrUnavailable {
		t.Fatal("retained recipient restored revoked group access", err)
	}
	in.Config.Period = "month"
	if _, err := s.SaveScheme(ctx, out.ID, conditioned(t, s, out.ID, in)); err != ErrInput {
		t.Fatal("new structural term accepted a recipient without group permission", err)
	}
}
