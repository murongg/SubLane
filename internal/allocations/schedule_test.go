package allocations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage/db"
)

func pendingInitial(t *testing.T) (*Service, int64, Scheme, SchemeInput) {
	t.Helper()
	s, _, user, _ := fixture(t)
	syntheticClock(s)
	in := SchemeInput{Name: "Synthetic initial schedule", GroupID: 2, Enabled: true, StartNext: true, Config: Config{Mode: "ratio", RatioUnit: "tokens", Period: "durations", Windows: []WindowCondition{{DurationSeconds: 18000, Limit: 100}}, Members: []Share{{UserID: user, Limit: 10000}}}}
	out, err := s.SaveScheme(context.Background(), 0, in)
	if err != nil {
		t.Fatal(err)
	}
	return s, user, out, in
}

func scheduleConditions(t *testing.T, s *Service, out Scheme, in SchemeInput, action string) Conditions {
	t.Helper()
	c := conditioned(t, s, out.ID, in).Conditions
	if action == "reschedule" {
		p, err := s.Preview(context.Background(), out.ID, action)
		if err != nil {
			t.Fatal(err)
		}
		c.ExpectedEffectiveAt, _ = json.Marshal(p.ProposedEffectiveAt)
	}
	return c
}

func TestCancelInitialPreservesReadableDataAndPauseStillWorks(t *testing.T) {
	s, _, out, in := pendingInitial(t)
	ctx := context.Background()
	for _, enabled := range []bool{true, false} {
		if err := s.SetEnabled(ctx, out.ID, enabled); err != nil {
			t.Fatal(err)
		}
		_, err := s.ChangeSchedule(ctx, out.ID, "cancel", scheduleConditions(t, s, out, in, "cancel"))
		if err != ErrInitialSchedule {
			t.Fatal("initial cancellation accepted", err)
		}
		rows, err := s.Schemes(ctx)
		if err != nil || len(rows) != 1 || rows[0].Next == nil || rows[0].Next.ID != out.Next.ID || rows[0].Config.Mode != "ratio" || rows[0].Enabled != enabled {
			t.Fatal("initial schedule lost", rows, err)
		}
	}
}

func TestInitialRescheduleUsesSuccessorAndRetainsRevision(t *testing.T) {
	s, _, out, in := pendingInitial(t)
	ctx := context.Background()
	old := out.Next.EffectiveAt
	updated, err := s.ChangeSchedule(ctx, out.ID, "reschedule", scheduleConditions(t, s, out, in, "reschedule"))
	if err != nil || updated.ID != out.ID || updated.Next == nil || updated.Next.EffectiveAt != old+18000 || updated.Next.AccountingAnchorAt != updated.Next.EffectiveAt || updated.WriteConditions.Current != nil {
		t.Fatal("invalid atomic initial replacement", updated, err)
	}
	var count int
	s.conn.QueryRow(`SELECT count(*) FROM allocation_revisions WHERE scheme_id=? AND effective_at=?`, out.ID, old).Scan(&count)
	if count != 0 {
		t.Fatal("old pending snapshot revived", count)
	}
	rows, err := s.Schemes(ctx)
	if err != nil || len(rows) != 1 || rows[0].Config.Mode != "ratio" {
		t.Fatal("replacement unreadable", rows, err)
	}
}

func TestInitialRescheduleRollsBackInsertedReplacement(t *testing.T) {
	s, _, out, in := pendingInitial(t)
	ctx := context.Background()
	c := scheduleConditions(t, s, out, in, "reschedule")
	if _, err := s.conn.Exec(`CREATE TRIGGER synthetic_reject_retirement BEFORE DELETE ON allocation_revisions BEGIN SELECT RAISE(ABORT,'synthetic retirement failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := s.ChangeSchedule(ctx, out.ID, "reschedule", c)
	if err == nil || !strings.Contains(err.Error(), "synthetic retirement failure") {
		t.Fatal("retirement did not fail", err)
	}
	var count int
	s.conn.QueryRow(`SELECT count(*) FROM allocation_revisions WHERE scheme_id=?`, out.ID).Scan(&count)
	if count != 1 {
		t.Fatal("partial replacement committed", count)
	}
	state, err := s.readSchemeCore(ctx, db.New(s.conn), out.ID, s.now().Unix())
	if err != nil || state.Next == nil || state.Next.ID != out.Next.ID {
		t.Fatal("rollback lost original", state, err)
	}
}

func TestInitialRescheduleRollsBackWhenActivationCrossesDuringTransaction(t *testing.T) {
	s, _, out, in := pendingInitial(t)
	c := scheduleConditions(t, s, out, in, "reschedule")
	start := s.now().Unix()
	calls := 0
	s.now = func() time.Time {
		calls++
		if calls >= 3 {
			return time.Unix(out.Next.EffectiveAt, 0)
		}
		return time.Unix(start, 0)
	}
	_, err := s.ChangeSchedule(context.Background(), out.ID, "reschedule", c)
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatal("boundary passed without rollback", err)
	}
	var count int
	s.conn.QueryRow(`SELECT count(*) FROM allocation_revisions WHERE scheme_id=?`, out.ID).Scan(&count)
	if count != 1 {
		t.Fatal("boundary failure committed", count)
	}
}

func TestCancelAfterCurrentExistsPreservesPausedCurrent(t *testing.T) {
	s, _, out, in := pendingInitial(t)
	now := out.Next.EffectiveAt + 1
	s.now = func() time.Time { return time.Unix(now, 0) }
	ctx := context.Background()
	in.StartNext = false
	in.Config.Windows[0].Limit = 120
	current, err := s.SaveScheme(ctx, out.ID, conditioned(t, s, out.ID, in))
	if err != nil || current.Next == nil {
		t.Fatal(current, err)
	}
	if err := s.SetEnabled(ctx, out.ID, false); err != nil {
		t.Fatal(err)
	}
	updated, err := s.ChangeSchedule(ctx, out.ID, "cancel", scheduleConditions(t, s, current, in, "cancel"))
	if err != nil || updated.Next != nil || updated.Enabled || updated.WriteConditions.Current == nil || updated.Revision.ID != out.Next.ID {
		t.Fatal("cancel removed current", updated, err)
	}
}

func TestReplacementPreviewBindsNewTermAndRejectsFakeReplacement(t *testing.T) {
	s, _, user, _ := fixture(t)
	now := syntheticClock(s)
	ctx := context.Background()
	in := SchemeInput{Name: "Synthetic replacement", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", RatioUnit: "tokens", Period: "durations", Windows: []WindowCondition{{DurationSeconds: 18000, Limit: 100}}, Members: []Share{{UserID: user, Limit: 10000}}}}
	out, err := s.SaveScheme(ctx, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Preview(ctx, out.ID, "replace")
	if err != nil || p.ProposedAnchorAt != p.ProposedEffectiveAt {
		t.Fatal("replacement preview not a new term", p, err)
	}
	fake := conditioned(t, s, out.ID, in)
	fake.ConfirmReplacement = true
	if _, err := s.SaveScheme(ctx, out.ID, fake); err != ErrReplacement {
		t.Fatal("same-shape reset accepted", err)
	}
	fake.ApplyNow = true
	if _, err := s.SaveScheme(ctx, out.ID, fake); err != ErrReplacement {
		t.Fatal("immediate member path ignored the new-term confirmation", err)
	}
	in.Config.Windows[0].DurationSeconds = 14400
	in = conditioned(t, s, out.ID, in)
	changed, err := s.SaveScheme(ctx, out.ID, in)
	if err != nil || changed.Next == nil || changed.Next.AccountingAnchorAt != p.ProposedEffectiveAt || changed.Next.EffectiveAt != p.ProposedEffectiveAt {
		t.Fatal("replacement anchor not bound", changed, err)
	}
	*now = p.ProposedEffectiveAt + 1
}

func TestNewScheduleFinalTimeFenceRollsBackAfterWrites(t *testing.T) {
	s, conn, user, _ := fixture(t)
	now := syntheticClock(s)
	start := *now
	ctx := context.Background()
	in := SchemeInput{Name: "Synthetic final deadline", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}}
	out, err := s.SaveScheme(ctx, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	in = conditioned(t, s, out.ID, in)
	calls := 0
	s.now = func() time.Time {
		calls++
		if calls >= 4 {
			return time.Unix(out.EditEffectiveAt, 0)
		}
		return time.Unix(start, 0)
	}
	_, err = s.SaveScheme(ctx, out.ID, in)
	if err != ErrEffectiveConflict {
		t.Fatal("expired new schedule committed", err)
	}
	var count int
	conn.QueryRow(`SELECT count(*) FROM allocation_revisions`).Scan(&count)
	if count != 1 {
		t.Fatal("expired write was not rolled back", count)
	}
}

func TestPostponementUsesCurrentNaturalBoundaryAndRetiresAllOldSnapshots(t *testing.T) {
	for _, structural := range []bool{false, true} {
		for _, oldHour := range []int64{168, 170} {
			t.Run(fmt.Sprintf("structural=%t/hour=%d", structural, oldHour), func(t *testing.T) {
				s, conn, user, _ := fixture(t)
				now := syntheticClock(s)
				start := *now
				ctx := context.Background()
				in := SchemeInput{Name: "Synthetic postponement", GroupID: 2, Enabled: true, Config: Config{Mode: "ratio", RatioUnit: "tokens", Period: "durations", Windows: []WindowCondition{{DurationSeconds: 5 * 3600, Limit: 100}, {DurationSeconds: 168 * 3600, Limit: 1000}}, Members: []Share{{UserID: user, Limit: 10000}}}}
				out, err := s.SaveScheme(ctx, 0, in)
				if err != nil {
					t.Fatal(err)
				}
				*now += 167 * 3600
				in.Config.Windows[0].Limit = 120
				if structural {
					in.Config.Windows[0].DurationSeconds = 4 * 3600
				}
				out, err = s.SaveScheme(ctx, out.ID, conditioned(t, s, out.ID, in))
				if err != nil || out.Next == nil {
					t.Fatal(out, err)
				}
				// A pre-upgrade timestamp may not lie on the current schedule's boundary.
				oldAt := start + oldHour*3600
				anchor := start
				if structural {
					anchor = oldAt
				}
				if _, err := conn.Exec(`UPDATE allocation_revisions SET effective_at=?,accounting_anchor_at=? WHERE id=?`, oldAt, anchor, out.Next.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := appendRevision(ctx, db.New(conn), out.ID, oldAt, anchor, out.Next.Config); err != nil {
					t.Fatal(err)
				}
				preview, err := s.Preview(ctx, out.ID, "reschedule")
				if err != nil || preview.ProposedEffectiveAt != start+336*3600 {
					t.Fatal("preview drifted from the current natural boundary", preview, err)
				}
				conditions := scheduleConditions(t, s, out, in, "reschedule")
				for _, rejectedHour := range []int64{168, 200, 504} {
					conditions.ExpectedEffectiveAt, _ = json.Marshal(start + rejectedHour*3600)
					if _, err := s.ChangeSchedule(ctx, out.ID, "reschedule", conditions); err != ErrEffectiveConflict {
						t.Fatal("arbitrary postponement accepted", rejectedHour, err)
					}
				}
				conditions.ExpectedEffectiveAt, _ = json.Marshal(preview.ProposedEffectiveAt)
				updated, err := s.ChangeSchedule(ctx, out.ID, "reschedule", conditions)
				if err != nil || updated.Next == nil || updated.Next.EffectiveAt != preview.ProposedEffectiveAt {
					t.Fatal("commit disagrees with preview", updated, err)
				}
				if structural {
					anchor = preview.ProposedEffectiveAt
				}
				if updated.Next.AccountingAnchorAt != anchor || preview.ProposedAnchorAt != anchor {
					t.Fatal("postponement changed the wrong accounting anchor", updated.Next, preview)
				}
				var retired int
				if err := conn.QueryRow(`SELECT count(*) FROM allocation_revisions WHERE scheme_id=? AND effective_at=?`, out.ID, oldAt).Scan(&retired); err != nil || retired != 0 {
					t.Fatal("superseded pending snapshot could revive", retired, err)
				}
				preview, err = s.Preview(ctx, out.ID, "reschedule")
				if err != nil || preview.ProposedEffectiveAt != start+504*3600 {
					t.Fatal("second explicit postponement does not advance one boundary", preview, err)
				}
			})
		}
	}
}

func TestCalendarPostponementUsesCivilBoundary(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{location: func() *time.Location { return location }}
	for _, test := range []struct {
		name       string
		config     Config
		next, want time.Time
	}{
		{"spring-forward", Config{Mode: "tokens", Period: "day", ResetTime: "02:30"}, time.Date(2027, 3, 14, 3, 0, 0, 0, location), time.Date(2027, 3, 15, 2, 30, 0, 0, location)},
		{"short-month", Config{Mode: "tokens", Period: "month", ResetDay: 31}, time.Date(2027, 2, 28, 0, 0, 0, 0, location), time.Date(2027, 3, 31, 0, 0, 0, 0, location)},
	} {
		t.Run(test.name, func(t *testing.T) {
			id := int64(1)
			state := Scheme{Revision: Revision{Config: test.config}, WriteConditions: WriteConditions{Current: &id}, Next: &Revision{Config: test.config, EffectiveAt: test.next.Unix()}}
			got, err := s.targetTime(state, "reschedule", test.next.Unix()-1)
			if err != nil || got != test.want.Unix() {
				t.Fatal("postponement used a fixed duration instead of a civil boundary", got, err)
			}
		})
	}
}
