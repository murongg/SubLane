package resets

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/storage"
)

const syntheticAccount = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type fixture struct {
	s           *Service
	db          *sql.DB
	now         time.Time
	observation Observation
	calls       int
	keys        []string
	outcome     string
	err         error
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	conn, err := storage.Open(ctx, filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	_, err = conn.Exec(`INSERT INTO users(id,username,role,password_hash,enabled,created_at) VALUES(1,'synthetic-admin','admin','synthetic-hash',1,1);
	INSERT INTO tenants(id,name,owner_user_id,created_at) VALUES(1,'Synthetic workspace',1,1);
	INSERT INTO memberships(tenant_id,user_id,role,created_at) VALUES(1,1,'owner',1);
	INSERT INTO accounts(id,name,account_id,status,credential,created_at,updated_at) VALUES('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','Synthetic account','synthetic-subject','ready',X'00',1,1);`)
	if err != nil {
		t.Fatal(err)
	}
	count := int64(3)
	f := &fixture{db: conn, now: time.Date(2026, 10, 29, 0, 59, 0, 0, time.UTC), observation: Observation{Fresh: true, Blocked: true, Credits: &count}, outcome: "reset"}
	f.observation.Cards = []Card{{ID: "synthetic-default-card", Status: "available", Supported: true}}
	f.s = New(conn, func() *time.Location { return time.FixedZone("Synthetic/UTC+8", 8*3600) }, func(context.Context, int64, string) (Observation, error) { return f.observation, nil }, func(_ context.Context, _ int64, _ string, key, card string) (string, error) {
		f.calls++
		f.keys = append(f.keys, key)
		return f.outcome, f.err
	})
	f.s.now = func() time.Time { return f.now }
	return f
}

func defaultConfig() Config {
	return Config{Frequency: "daily", Time: "09:00", Weekday: 1, DailyLimit: 1}
}

func TestScheduledAndExhaustedShareDailyLimitAcrossRestart(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	cfg := defaultConfig()
	cfg.Scheduled = true
	cfg.Exhausted = true
	state, err := f.s.Save(ctx, 1, syntheticAccount, cfg)
	if err != nil || state.NextRunAt != f.now.Add(time.Minute).Unix() {
		t.Fatal(state, err)
	}
	if _, err = f.s.Run(ctx, 1, syntheticAccount, "check"); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatal("exhaustion did not redeem", f.calls)
	}
	f.now = f.now.Add(time.Minute)
	if _, err = f.s.Run(ctx, 1, syntheticAccount, "check"); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatal("scheduled trigger exceeded shared daily limit")
	}
	state, err = f.s.State(ctx, 1, syntheticAccount)
	if err != nil || state.UsedToday != 1 || len(state.History) < 2 || state.History[0].Reason != "daily_limit" {
		t.Fatal(state, err)
	}
	other := New(f.db, f.s.location, f.s.inspect, f.s.redeem)
	other.now = f.s.now
	if _, err = other.Run(ctx, 1, syntheticAccount, "manual"); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatal("restart lost daily usage")
	}
	f.now = f.now.Add(24 * time.Hour)
	if _, err = other.Run(ctx, 1, syntheticAccount, "check"); err != nil {
		t.Fatal(err)
	}
	if f.calls != 2 {
		t.Fatal("next day never resumed", f.calls)
	}
}

func TestAmbiguousOutcomeBlocksNewAttemptsAndReplaysPersistedID(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.err = errors.New("synthetic transport timeout")
	state, err := f.s.Run(ctx, 1, syntheticAccount, "manual")
	if err != nil || !state.Pending || f.calls != 1 {
		t.Fatal(state, err)
	}
	other := New(f.db, f.s.location, f.s.inspect, f.s.redeem)
	other.now = f.s.now
	if _, err = other.Run(ctx, 1, syntheticAccount, "manual"); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatal("new ID spent while prior outcome unknown")
	}
	f.err = nil
	f.outcome = "already_redeemed"
	state, err = other.Retry(ctx, 1, syntheticAccount)
	if err != nil || state.Pending || state.UsedToday != 1 || f.calls != 2 || f.keys[0] != f.keys[1] {
		t.Fatal(state, f.keys, err)
	}
}

func TestChecksNeverConsumeUnknownOrUnblockedQuota(t *testing.T) {
	for _, tc := range []struct {
		name           string
		fresh, blocked bool
		credits        *int64
	}{
		{"stale", false, true, new(int64(2))}, {"available", true, false, new(int64(2))}, {"unknown cards", true, true, nil}, {"no cards", true, true, new(int64(0))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			cfg := defaultConfig()
			cfg.Exhausted = true
			if _, err := f.s.Save(context.Background(), 1, syntheticAccount, cfg); err != nil {
				t.Fatal(err)
			}
			f.observation = Observation{Fresh: tc.fresh, Blocked: tc.blocked, Credits: tc.credits}
			if _, err := f.s.Run(context.Background(), 1, syntheticAccount, "check"); err != nil {
				t.Fatal(err)
			}
			if f.calls != 0 {
				t.Fatal("unsafe automatic consumption")
			}
		})
	}
}

func TestMissedScheduleSkipsWithoutCatchupAndOnceDoesNotRepeat(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	cfg := defaultConfig()
	cfg.Scheduled = true
	cfg.Frequency = "once"
	cfg.Date = "2026-10-29"
	if _, err := f.s.Save(ctx, 1, syntheticAccount, cfg); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(10 * time.Minute)
	state, err := f.s.Run(ctx, 1, syntheticAccount, "check")
	if err != nil || f.calls != 0 || state.NextRunAt != 0 || state.History[0].Reason != "missed" {
		t.Fatal(state, err)
	}
	if _, err := f.s.Run(ctx, 1, syntheticAccount, "check"); err != nil || f.calls != 0 {
		t.Fatal(err)
	}
}

func TestLifecycleChangesDuringInspectionPreventRedemption(t *testing.T) {
	for _, change := range []string{"UPDATE accounts SET enabled=0", "UPDATE accounts SET models_revision=models_revision+1", "UPDATE accounts SET status='reauth_required'"} {
		t.Run(change, func(t *testing.T) {
			f := setup(t)
			f.s.inspect = func(context.Context, int64, string) (Observation, error) {
				_, err := f.db.Exec(change)
				return f.observation, err
			}
			if _, err := f.s.Run(context.Background(), 1, syntheticAccount, "manual"); err != nil {
				t.Fatal(err)
			}
			if f.calls != 0 {
				t.Fatal("lifecycle changed while reading quota")
			}
		})
	}
}

func TestConcurrentTriggersHaveOnePersistentClaim(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	var calls atomic.Int32
	f.s.redeem = func(context.Context, int64, string, string, string) (string, error) {
		calls.Add(1)
		close(entered)
		<-release
		return "reset", nil
	}
	go func() { _, err := f.s.Run(ctx, 1, syntheticAccount, "manual"); done <- err }()
	<-entered
	other := New(f.db, f.s.location, f.s.inspect, f.s.redeem)
	other.now = f.s.now
	if _, err := other.Run(ctx, 1, syntheticAccount, "manual"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil || calls.Load() != 1 {
		t.Fatal(err, calls.Load())
	}
}

func TestSettingsAreScopedValidatedAndDefaultOff(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	state, err := f.s.State(ctx, 1, syntheticAccount)
	if err != nil || state.Config.Scheduled || state.Config.Exhausted || state.Config.DailyLimit != 1 {
		t.Fatal(state, err)
	}
	if _, err := f.s.State(ctx, 2, syntheticAccount); err == nil {
		t.Fatal("foreign account readable")
	}
	bad := defaultConfig()
	bad.Scheduled = true
	bad.Time = "25:00"
	if _, err := f.s.Save(ctx, 1, syntheticAccount, bad); !errors.Is(err, ErrInput) {
		t.Fatal(err)
	}
	bad = defaultConfig()
	bad.DailyLimit = 0
	if _, err := f.s.Save(ctx, 1, syntheticAccount, bad); !errors.Is(err, ErrInput) {
		t.Fatal(err)
	}
}

func TestCivilSchedulesRespectWeeklyRulesAndDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Scheduled = true
	cfg.Time = "02:30"
	after := time.Date(2026, 3, 7, 3, 0, 0, 0, loc)
	next := nextOccurrence(cfg, after, loc)
	if next.Format("2006-01-02 15:04") != "2026-03-09 02:30" {
		t.Fatal("nonexistent civil time must be skipped", next)
	}
	cfg.Frequency = "weekly"
	cfg.Weekday = int(time.Monday)
	cfg.Time = "09:00"
	if next := nextOccurrence(cfg, after, loc); next.Weekday() != time.Monday || next.Hour() != 9 {
		t.Fatal(next)
	}
	cfg.Frequency = "daily"
	cfg.Time = "01:30"
	first := nextOccurrence(cfg, time.Date(2026, 11, 1, 0, 0, 0, 0, loc), loc)
	next = nextOccurrence(cfg, first, loc)
	if next.Day() != 2 {
		t.Fatal("repeated DST hour created two daily runs", first, next)
	}
}

func TestWorkerRunsWithoutBrowserAndJoinsOnShutdown(t *testing.T) {
	f := setup(t)
	cfg := defaultConfig()
	cfg.Exhausted = true
	if _, err := f.s.Save(context.Background(), 1, syntheticAccount, cfg); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	f.s.redeem = func(ctx context.Context, _ int64, _ string, _ string, _ string) (string, error) {
		close(entered)
		<-ctx.Done()
		return "", ctx.Err()
	}
	f.s.Start(context.Background())
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("idle account never checked")
	}
	f.s.Close()
	state, err := f.s.State(context.Background(), 1, syntheticAccount)
	if err != nil || !state.Pending {
		t.Fatal("shutdown lost uncertain attempt", state, err)
	}
}

func TestReplayLifecycleErrorCannotClearAnUncertainSpend(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.err = errors.New("synthetic lost response")
	if _, err := f.s.Run(ctx, 1, syntheticAccount, "manual"); err != nil {
		t.Fatal(err)
	}
	f.err = accounts.ErrDisabled
	state, err := f.s.Retry(ctx, 1, syntheticAccount)
	if err != nil || !state.Pending || state.UsedToday != 1 {
		t.Fatal("local lifecycle error erased an unresolved provider attempt", state, err)
	}
	if _, err := f.s.Run(ctx, 1, syntheticAccount, "manual"); err != nil || f.calls != 2 {
		t.Fatal("new request escaped pending reservation", f.calls, err)
	}
}

func TestOnceDSTGapNeverMovesToAnotherDateAndWeeklyResumesAfterGap(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Scheduled = true
	cfg.Frequency = "once"
	cfg.Time = "02:30"
	cfg.Date = "2026-03-08"
	if next := nextOccurrence(cfg, time.Date(2026, 3, 7, 12, 0, 0, 0, loc), loc); !next.IsZero() {
		t.Fatal("one-time date silently moved", next)
	}
	cfg.Frequency = "weekly"
	cfg.Weekday = int(time.Sunday)
	if next := nextOccurrence(cfg, time.Date(2026, 3, 1, 2, 30, 0, 0, loc), loc); next.Format("2006-01-02 15:04") != "2026-03-15 02:30" {
		t.Fatal("weekly rule stopped at DST gap", next)
	}
}

func TestCompletedOnceRuleCanKeepItsDateWhenEditingOtherSettings(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	cfg := defaultConfig()
	cfg.Scheduled = true
	cfg.Frequency = "once"
	cfg.Date = "2026-10-29"
	if _, err := f.s.Save(ctx, 1, syntheticAccount, cfg); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	if _, err := f.s.Run(ctx, 1, syntheticAccount, "check"); err != nil {
		t.Fatal(err)
	}
	f.s.location = func() *time.Location { return time.FixedZone("Synthetic/UTC+7", 7*3600) }
	if state, err := f.s.State(ctx, 1, syntheticAccount); err != nil || state.NextRunAt != 0 {
		t.Fatal("time zone change resurrected completed schedule", state, err)
	}
	cfg.DailyLimit = 2
	cfg.Exhausted = true
	state, err := f.s.Save(ctx, 1, syntheticAccount, cfg)
	if err != nil || state.NextRunAt != 0 || !state.Config.Exhausted {
		t.Fatal(state, err)
	}
}

func TestDailyCapUsesActualCivilMidnights(t *testing.T) {
	for _, tc := range []struct {
		zone  string
		year  int
		month time.Month
		day   int
		hours time.Duration
	}{
		{"Africa/Cairo", 2026, time.April, 24, 23},
		{"America/New_York", 2026, time.March, 8, 23},
		{"America/New_York", 2026, time.November, 1, 25},
	} {
		t.Run(tc.zone+tc.month.String(), func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			noon := time.Date(tc.year, tc.month, tc.day, 12, 0, 0, 0, loc)
			start, end := dayBounds(noon, loc)
			if time.Duration(end-start)*time.Second != tc.hours*time.Hour {
				t.Fatal("wrong local-day budget window", time.Unix(start, 0).In(loc), time.Unix(end, 0).In(loc))
			}
			if time.Unix(start-1, 0).In(loc).Day() == tc.day || time.Unix(end, 0).In(loc).Day() == tc.day {
				t.Fatal("boundary excludes or includes part of the local day")
			}
		})
	}
}

func TestWeeklyScheduleKeepsItsDateAcrossAMidnightGap(t *testing.T) {
	loc, err := time.LoadLocation("Africa/Cairo")
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Scheduled = true
	cfg.Frequency = "weekly"
	cfg.Weekday = int(time.Friday)
	if next := nextOccurrence(cfg, time.Date(2026, 4, 23, 0, 30, 0, 0, loc), loc); next.Format("2006-01-02 15:04") != "2026-04-24 09:00" {
		t.Fatal("valid scheduled date was lost to an unrelated midnight gap", next)
	}
}
