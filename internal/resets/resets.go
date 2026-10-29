// Package resets owns opt-in reset-credit rules and durable, bounded execution history.
package resets

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
)

var (
	ErrInput       = errors.New("invalid_reset_settings")
	ErrBusy        = errors.New("reset_busy")
	ErrNoPending   = errors.New("reset_not_pending")
	ErrUnsupported = errors.New("reset_unsupported")
)

type Config struct {
	Scheduled  bool   `json:"scheduled"`
	Exhausted  bool   `json:"exhausted"`
	Frequency  string `json:"frequency"`
	Time       string `json:"time"`
	Date       string `json:"date"`
	Weekday    int    `json:"weekday"`
	DailyLimit int64  `json:"daily_limit"`
}

type Run struct {
	Trigger   string `json:"trigger"`
	Outcome   string `json:"outcome"`
	Reason    string `json:"reason"`
	StartedAt int64  `json:"started_at"`
}

type State struct {
	Config    Config `json:"config"`
	NextRunAt int64  `json:"next_run_at"`
	TimeZone  string `json:"time_zone"`
	UsedToday int64  `json:"used_today"`
	Pending   bool   `json:"pending"`
	History   []Run  `json:"history"`
}

// Observation comes from the shared quota cache; provider payloads and credentials stay outside this domain.
type Observation struct {
	Fresh, Blocked bool
	Credits        *int64
	Cards          []Card
}
type Inspector func(context.Context, int64, string) (Observation, error)
type Redeemer func(context.Context, int64, string, string, string) (string, error)

type Service struct {
	conn     *sql.DB
	queries  *db.Queries
	location func() *time.Location
	inspect  Inspector
	redeem   Redeemer
	now      func() time.Time
	mu       sync.Mutex
	busy     map[string]bool
	cancel   context.CancelFunc
	workers  sync.WaitGroup
	closed   bool
}

func New(conn *sql.DB, location func() *time.Location, inspect Inspector, redeem Redeemer) *Service {
	if location == nil {
		location = func() *time.Location { return time.UTC }
	}
	return &Service{conn: conn, queries: db.New(conn), location: location, inspect: inspect, redeem: redeem, now: time.Now, busy: make(map[string]bool)}
}

func config(raw string) (Config, error) {
	c := Config{Frequency: "daily", Time: "09:00", Weekday: 1, DailyLimit: 1}
	if json.Unmarshal([]byte(raw), &c) != nil {
		return c, ErrInput
	}
	return c, nil
}

func (s *Service) account(ctx context.Context, tenant int64, id string) (db.GetResetAccountRow, error) {
	row, err := s.queries.GetResetAccount(ctx, db.GetResetAccountParams{ID: id, TenantID: tenant})
	if errors.Is(err, sql.ErrNoRows) {
		err = accounts.ErrNotFound
	}
	if err == nil && row.Provider != "codex" {
		err = ErrUnsupported
	}
	return row, err
}

func (s *Service) lock(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.busy[id] {
		return false
	}
	s.busy[id] = true
	return true
}
func (s *Service) unlock(id string) { s.mu.Lock(); delete(s.busy, id); s.mu.Unlock() }

func (s *Service) State(ctx context.Context, tenant int64, id string) (State, error) {
	row, err := s.account(ctx, tenant, id)
	if err != nil {
		return State{}, err
	}
	c, err := config(row.ResetConfig)
	if err != nil {
		return State{}, err
	}
	state := State{Config: c, NextRunAt: row.ResetNextRunAt, TimeZone: s.location().String(), History: []Run{}}
	if row.ResetTimeZone != state.TimeZone && !(c.Frequency == "once" && row.ResetNextRunAt == 0) {
		state.NextRunAt = timestamp(nextOccurrence(c, s.now(), s.location()))
	}
	start, end := dayBounds(s.now(), s.location())
	state.UsedToday, err = s.queries.CountResetUsage(ctx, db.CountResetUsageParams{AccountID: id, DayStart: start, DayEnd: end})
	if err != nil {
		return State{}, err
	}
	_, err = s.queries.PendingResetRun(ctx, id)
	state.Pending = err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return State{}, err
	}
	runs, err := s.queries.ListResetRuns(ctx, id)
	if err != nil {
		return State{}, err
	}
	for _, run := range runs {
		state.History = append(state.History, Run{Trigger: run.TriggerKind, Outcome: run.Outcome, Reason: run.Reason, StartedAt: run.StartedAt})
	}
	return state, nil
}

func (s *Service) Save(ctx context.Context, tenant int64, id string, c Config) (State, error) {
	if !s.lock(id) {
		return State{}, ErrBusy
	}
	defer s.unlock(id)
	row, err := s.account(ctx, tenant, id)
	if err != nil {
		return State{}, err
	}
	if c.DailyLimit < 1 || c.DailyLimit > 100 || c.Weekday < 0 || c.Weekday > 6 || len(c.Date) > 10 || (c.Frequency != "once" && c.Frequency != "daily" && c.Frequency != "weekly") {
		return State{}, ErrInput
	}
	if _, err := time.Parse("15:04", c.Time); err != nil || len(c.Time) != 5 {
		return State{}, ErrInput
	}
	next := timestamp(nextOccurrence(c, s.now(), s.location()))
	previous, err := config(row.ResetConfig)
	if err != nil {
		return State{}, err
	}
	completedOnce := previous.Scheduled && previous.Frequency == "once" && row.ResetNextRunAt == 0 && previous.Date == c.Date && previous.Time == c.Time
	if completedOnce && c.Frequency == "once" {
		next = 0
	}
	if c.Scheduled && c.Frequency == "once" && next == 0 {
		if !completedOnce {
			return State{}, ErrInput
		}
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return State{}, err
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	n, err := q.SaveResetConfig(ctx, db.SaveResetConfigParams{ID: id, TenantID: tenant, Config: string(raw), Revision: row.ResetRevision, NextAt: next, Zone: s.location().String()})
	if err != nil {
		return State{}, err
	}
	if n != 1 {
		return State{}, ErrBusy
	}
	if err := audit.Record(ctx, q, "account.reset_settings", "account", id); err != nil {
		return State{}, err
	}
	if err := tx.Commit(); err != nil {
		return State{}, err
	}
	return s.State(ctx, tenant, id)
}

func (s *Service) Run(ctx context.Context, tenant int64, id, trigger string) (State, error) {
	if trigger != "manual" && trigger != "check" {
		return State{}, ErrInput
	}
	if !s.lock(id) {
		return State{}, ErrBusy
	}
	defer s.unlock(id)
	row, err := s.account(ctx, tenant, id)
	if err != nil {
		return State{}, err
	}
	c, err := config(row.ResetConfig)
	if err != nil {
		return State{}, err
	}
	if _, err = s.queries.PendingResetRun(ctx, id); err == nil {
		return s.State(ctx, tenant, id)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return State{}, err
	}
	now := s.now()
	var occurrence *int64
	if trigger == "check" {
		if row.ResetTimeZone != s.location().String() {
			next := timestamp(nextOccurrence(c, now, s.location()))
			if c.Frequency == "once" && row.ResetNextRunAt == 0 {
				next = 0
			}
			n, err := s.queries.AdvanceResetSchedule(ctx, db.AdvanceResetScheduleParams{ID: id, TenantID: tenant, Revision: row.ResetRevision, PreviousAt: row.ResetNextRunAt, NextAt: next, Zone: s.location().String()})
			if err != nil {
				return State{}, err
			}
			if n != 1 {
				return State{}, ErrBusy
			}
			row.ResetNextRunAt = next
		}
		if c.Scheduled && row.ResetNextRunAt > 0 && row.ResetNextRunAt <= now.Unix() {
			trigger = "scheduled"
			at := row.ResetNextRunAt
			occurrence = &at
			n, err := s.queries.AdvanceResetSchedule(ctx, db.AdvanceResetScheduleParams{ID: id, TenantID: tenant, Revision: row.ResetRevision, PreviousAt: at, NextAt: timestamp(nextOccurrence(c, now, s.location())), Zone: s.location().String()})
			if err != nil {
				return State{}, err
			}
			if n != 1 {
				return s.State(ctx, tenant, id)
			}
			if now.Unix()-at > 300 {
				return s.skip(ctx, tenant, id, trigger, occurrence, "missed")
			}
		} else if c.Exhausted {
			trigger = "exhausted"
		} else {
			return s.State(ctx, tenant, id)
		}
	}
	if !row.Enabled {
		return s.skip(ctx, tenant, id, trigger, occurrence, "disabled")
	}
	if row.Status == "reauth_required" {
		return s.skip(ctx, tenant, id, trigger, occurrence, "reauthorize")
	}
	if s.inspect == nil || s.redeem == nil {
		return State{}, ErrBusy
	}
	observation, err := s.inspect(ctx, tenant, id)
	if err != nil {
		return s.skip(ctx, tenant, id, trigger, occurrence, "usage_unavailable")
	}
	if !observation.Fresh {
		return s.skip(ctx, tenant, id, trigger, occurrence, "quota_unknown")
	}
	if trigger == "exhausted" && !observation.Blocked {
		return s.skip(ctx, tenant, id, trigger, occurrence, "quota_available")
	}
	if observation.Credits == nil {
		return s.skip(ctx, tenant, id, trigger, occurrence, "quota_unknown")
	}
	if *observation.Credits <= 0 {
		return s.skip(ctx, tenant, id, trigger, occurrence, "no_credit")
	}
	// An unavailable inventory is different from a known empty list; neither permits an unspecified spend.
	now = s.now()
	if observation.Cards == nil {
		return s.skip(ctx, tenant, id, trigger, occurrence, "cards_unavailable")
	}
	selected := PrepareCards(CardList{Cards: observation.Cards}, now).NextID
	if selected == "" {
		return s.skip(ctx, tenant, id, trigger, occurrence, "no_eligible_credit")
	}

	// The reservation commits before provider IO: a crash or another trigger cannot create a new attempt.
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	current, err := q.GetResetAccount(ctx, db.GetResetAccountParams{ID: id, TenantID: tenant})
	if err != nil {
		return State{}, err
	}
	if !current.Enabled || current.Status == "reauth_required" || current.ModelsRevision != row.ModelsRevision || current.ResetRevision != row.ResetRevision {
		tx.Rollback()
		return s.skip(ctx, tenant, id, trigger, occurrence, "account_changed")
	}
	if _, err := q.PendingResetRun(ctx, id); err == nil {
		tx.Rollback()
		return s.State(ctx, tenant, id)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return State{}, err
	}
	start, end := dayBounds(now, s.location())
	used, err := q.CountResetUsage(ctx, db.CountResetUsageParams{AccountID: id, DayStart: start, DayEnd: end})
	if err != nil {
		return State{}, err
	}
	if used >= c.DailyLimit {
		tx.Rollback()
		return s.skip(ctx, tenant, id, trigger, occurrence, "daily_limit")
	}
	key := rand.Text()
	n, err := q.CreateResetRun(ctx, db.CreateResetRunParams{RequestID: key, AccountID: id, TriggerKind: trigger, Occurrence: occurrence, Outcome: "pending", Reason: "confirmation_required", Now: now.Unix(), CreditID: selected})
	if err != nil {
		return State{}, err
	}
	if n != 1 {
		tx.Rollback()
		return s.State(ctx, tenant, id)
	}
	if err := tx.Commit(); err != nil {
		return State{}, err
	}
	return s.consume(ctx, tenant, id, key, selected)
}

func (s *Service) skip(ctx context.Context, tenant int64, id, trigger string, occurrence *int64, reason string) (State, error) {
	// Repeated background observations share one visible status; scheduled occurrences remain individually traceable.
	if trigger == "exhausted" {
		runs, err := s.queries.ListResetRuns(ctx, id)
		if err != nil {
			return State{}, err
		}
		if len(runs) > 0 && runs[0].TriggerKind == trigger && runs[0].Reason == reason {
			return s.State(ctx, tenant, id)
		}
	}
	_, err := s.queries.CreateResetRun(ctx, db.CreateResetRunParams{RequestID: rand.Text(), AccountID: id, TriggerKind: trigger, Occurrence: occurrence, Outcome: "skipped", Reason: reason, Now: s.now().Unix()})
	if err != nil {
		return State{}, err
	}
	if err := s.prune(ctx, id); err != nil {
		return State{}, err
	}
	return s.State(ctx, tenant, id)
}

func (s *Service) Retry(ctx context.Context, tenant int64, id string) (State, error) {
	if !s.lock(id) {
		return State{}, ErrBusy
	}
	defer s.unlock(id)
	row, err := s.account(ctx, tenant, id)
	if err != nil {
		return State{}, err
	}
	if !row.Enabled {
		return State{}, accounts.ErrDisabled
	}
	if row.Status == "reauth_required" {
		return State{}, accounts.ErrReauthorize
	}
	run, err := s.queries.PendingResetRun(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return State{}, ErrNoPending
	}
	if err != nil {
		return State{}, err
	}
	// Replaying the persisted key is the only way to resolve uncertainty; card counts cannot prove consumption.
	return s.consume(ctx, tenant, id, run.RequestID, run.CreditID)
}

func (s *Service) consume(ctx context.Context, tenant int64, id, key, cardID string) (State, error) {
	// Keep the selected card paired with its idempotency key, even if its expiry or inventory changes.
	result, err := s.redeem(ctx, tenant, id, key, cardID)
	// A local replay failure cannot prove that the original request did not spend a card.
	status, reason := "pending", "confirmation_required"
	if err == nil {
		switch result {
		case "reset", "already_redeemed":
			status, reason = "success", result
		case "nothing_to_reset", "no_credit":
			status, reason = "skipped", result
		}
	}
	// A disconnected browser or process cancellation must not discard a known provider result.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	tx, err := s.conn.BeginTx(writeCtx, nil)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	_, err = q.FinishResetRun(writeCtx, db.FinishResetRunParams{RequestID: key, Outcome: status, Reason: reason, Now: s.now().Unix()})
	if err != nil {
		return State{}, err
	}
	if status == "success" {
		if err := audit.Record(writeCtx, q, "account.reset_use", "account", id); err != nil {
			return State{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return State{}, err
	}
	if err := s.prune(writeCtx, id); err != nil {
		return State{}, err
	}
	return s.State(writeCtx, tenant, id)
}

func (s *Service) prune(ctx context.Context, id string) error {
	start, _ := dayBounds(s.now(), s.location())
	return s.queries.PruneResetRuns(ctx, db.PruneResetRunsParams{AccountID: id, DayStart: start})
}
