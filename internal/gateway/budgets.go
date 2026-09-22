package gateway

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage/db"
)

var (
	ErrTokenQuota       = errors.New("token_quota_exceeded")
	ErrTokenPending     = errors.New("token_usage_pending")
	ErrTokenAccounting  = errors.New("token_accounting_unavailable")
	ErrBudgetLimit      = errors.New("token_budget_rule_limit")
	ErrBudgetSettlement = errors.New("invalid_token_settlement")
)

type BudgetError struct {
	Cause      error
	BudgetID   int64  `json:"budget_id"`
	GroupID    int64  `json:"group_id"`
	Model      string `json:"model"`
	Period     string `json:"period"`
	ResetAt    int64  `json:"reset_at"`
	RetryAfter int64  `json:"-"`
}

func (e *BudgetError) Error() string { return e.Cause.Error() }
func (e *BudgetError) Unwrap() error { return e.Cause }

type BudgetInput struct {
	GroupID int64  `json:"group_id"`
	Model   string `json:"model"`
	Period  string `json:"period"`
	Limit   int64  `json:"limit"`
	Enabled bool   `json:"enabled"`
}
type Budget struct {
	BudgetInput
	ID          int64  `json:"id"`
	GroupName   string `json:"group_name"`
	CreatedAt   int64  `json:"created_at"`
	Used        int64  `json:"used"`
	Pending     int64  `json:"pending"`
	WindowStart int64  `json:"window_start"`
	ResetAt     int64  `json:"reset_at"`
}
type BudgetPending struct {
	RequestID   string `json:"request_id"`
	StartedAt   int64  `json:"started_at"`
	KnownTokens int64  `json:"known_tokens"`
}
type BudgetPage struct {
	Rules   []Budget        `json:"rules"`
	Pending []BudgetPending `json:"pending"`
}

func budgetWindow(now time.Time, period string) (int64, int64) {
	now = now.UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if period == "month" {
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start.Unix(), start.AddDate(0, 1, 0).Unix()
	}
	return start.Unix(), start.AddDate(0, 0, 1).Unix()
}

// Called under s.mu. A previous process may have died after dispatch but before usage arrived.
// Durable active entries must become pending, never silently become free requests.
func (s *Service) recoverBudgets(ctx context.Context) error {
	if s.budgetFailure {
		return ErrTokenAccounting
	}
	if s.budgetsReady {
		return nil
	}
	if err := s.queries.RecoverTokenBudgetEntries(ctx); err != nil {
		return ErrTokenAccounting
	}
	if err := s.queries.RecoverAllocationEntries(ctx); err != nil {
		return ErrTokenAccounting
	}
	s.budgetsReady = true
	return nil
}
func readBudgets(ctx context.Context, q *db.Queries, user int64, now time.Time) ([]Budget, error) {
	rows, err := q.ListTokenBudgets(ctx, user)
	if err != nil {
		return nil, err
	}
	result := make([]Budget, 0, len(rows))
	for _, r := range rows {
		start, end := budgetWindow(now, r.Period)
		used, err := q.GetTokenBudgetUsage(ctx, db.GetTokenBudgetUsageParams{BudgetID: r.ID, WindowStart: start})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		pending, err := q.CountTokenBudgetPending(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, Budget{BudgetInput: BudgetInput{GroupID: r.GroupID, Model: r.Model, Period: r.Period, Limit: r.TokenLimit, Enabled: r.Enabled == 1}, ID: r.ID, GroupName: r.GroupName, CreatedAt: r.CreatedAt, Used: used, Pending: pending, WindowStart: start, ResetAt: end})
	}
	return result, nil
}
func (s *Service) Budgets(ctx context.Context, user int64) (BudgetPage, error) {
	page := BudgetPage{Rules: []Budget{}, Pending: []BudgetPending{}}
	if user <= 0 {
		return page, accounts.ErrInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.recoverBudgets(ctx); err != nil {
		return page, err
	}
	if _, err := s.queries.GetMemberLimits(ctx, user); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return page, auth.ErrMemberNotFound
		}
		return page, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return page, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	page.Rules, err = readBudgets(ctx, q, user, s.now())
	if err != nil {
		return page, err
	}
	pending, err := q.ListPendingTokenRequests(ctx, user)
	if err != nil {
		return page, err
	}
	for _, r := range pending {
		page.Pending = append(page.Pending, BudgetPending{RequestID: r.RequestID, StartedAt: r.StartedAt, KnownTokens: r.KnownTokens})
	}
	if err := pruneBudgets(ctx, q, s.now()); err != nil {
		return page, err
	}
	return page, tx.Commit()
}
func (s *Service) SaveBudget(ctx context.Context, user int64, input BudgetInput) (Budget, error) {
	var empty Budget
	if user <= 1 || input.GroupID < 0 || input.Limit < 1 || input.Limit > 1_000_000_000_000 || (input.Period != "day" && input.Period != "month") {
		return empty, accounts.ErrInput
	}
	if input.Model != "" {
		if !safeModel.MatchString(input.Model) {
			return empty, accounts.ErrInput
		}
		_, input.Model = groups.SplitModel(input.Model)
		if input.Model == "" || len(input.Model) > 128 {
			return empty, accounts.ErrInput
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.recoverBudgets(ctx); err != nil {
		return empty, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	if _, err := q.GetMember(ctx, user); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return empty, auth.ErrMemberNotFound
		}
		return empty, err
	}
	if input.GroupID != 0 {
		if _, err := q.GetGroup(ctx, input.GroupID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return empty, accounts.ErrInput
			}
			return empty, err
		}
	}
	rules, err := q.ListTokenBudgets(ctx, user)
	if err != nil {
		return empty, err
	}
	exists := false
	for _, r := range rules {
		if r.GroupID == input.GroupID && r.Model == input.Model && r.Period == input.Period {
			exists = true
		}
	}
	if !exists && len(rules) >= 64 {
		return empty, ErrBudgetLimit
	}
	enabled := int64(0)
	if input.Enabled {
		enabled = 1
	}
	id, err := q.SaveTokenBudget(ctx, db.SaveTokenBudgetParams{UserID: user, GroupID: input.GroupID, Model: input.Model, Period: input.Period, TokenLimit: input.Limit, Enabled: enabled, CreatedAt: s.now().Unix()})
	if err != nil {
		return empty, err
	}
	if err := audit.Record(ctx, q, "member.budget", "member", audit.ID(user)); err != nil {
		return empty, err
	}
	result, err := readBudgets(ctx, q, user, s.now())
	if err != nil {
		return empty, err
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	for _, r := range result {
		if r.ID == id {
			return r, nil
		}
	}
	return empty, ErrTokenAccounting
}

// Admission and settlement share s.mu, so a completed charge is visible before the next admission.
// Active generations may finish beyond the limit: this is an admission budget, not a generation cap.
func (s *Service) admitBudget(ctx context.Context, e *observation, model string) error {
	if err := s.recoverBudgets(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ErrTokenAccounting
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	scheme, err := allocations.KeyScheme(ctx, q, e.record.KeyID, e.record.UserID, e.record.GroupID, s.now().Unix())
	if err != nil {
		return err
	}
	e.schemeID = scheme
	if scheme != 0 {
		return tx.Commit()
	}
	rules, err := readBudgets(ctx, q, e.record.UserID, e.started)
	if err != nil {
		return ErrTokenAccounting
	}
	pending, err := q.ListPendingTokenRequests(ctx, e.record.UserID)
	if err != nil {
		return ErrTokenAccounting
	}
	// Bound unresolved debt even when all of the user's rules have been disabled.
	if len(pending) >= 248 {
		return ErrTokenPending
	}
	matched := make([]Budget, 0, len(rules))
	for _, b := range rules {
		if (b.GroupID != 0 && b.GroupID != e.record.GroupID) || (b.Model != "" && b.Model != model) {
			continue
		}
		if b.Enabled {
			failure := &BudgetError{BudgetID: b.ID, GroupID: b.GroupID, Model: b.Model, Period: b.Period, ResetAt: b.ResetAt, RetryAfter: max(1, b.ResetAt-s.now().Unix())}
			if b.Pending > 0 {
				failure.Cause = ErrTokenPending
				failure.ResetAt = 0
				return failure
			}
			if b.Used >= b.Limit {
				failure.Cause = ErrTokenQuota
				return failure
			}
		}
		matched = append(matched, b)
	}
	for _, b := range matched {
		if err := q.BeginTokenBudgetUsage(ctx, db.BeginTokenBudgetUsageParams{BudgetID: b.ID, WindowStart: b.WindowStart, WindowEnd: b.ResetAt}); err != nil {
			return ErrTokenAccounting
		}
		if err := q.BeginTokenBudgetEntry(ctx, db.BeginTokenBudgetEntryParams{RequestID: e.record.RequestID, BudgetID: b.ID, WindowStart: b.WindowStart, StartedAt: e.record.StartedAt}); err != nil {
			return ErrTokenAccounting
		}
	}
	if err := tx.Commit(); err != nil {
		return ErrTokenAccounting
	}
	e.budgetTracked = len(matched) > 0
	return nil
}
func (e *observation) settleBudget(ctx context.Context, q *db.Queries) error {
	if !e.budgetTracked {
		return nil
	}
	entries, err := q.ListTokenBudgetEntries(ctx, db.ListTokenBudgetEntriesParams{RequestID: e.record.RequestID, UserID: e.record.UserID})
	if err != nil {
		return err
	}
	tokens := int64(0)
	if e.record.InputTokens != nil {
		tokens += *e.record.InputTokens
	}
	if e.record.OutputTokens != nil {
		tokens += *e.record.OutputTokens
	}
	state := "settled"
	if e.budgetDispatched && (e.record.InputTokens == nil || e.record.OutputTokens == nil || (e.record.Outcome != "success" && e.record.Outcome != "incomplete")) {
		state = "pending"
	}
	// Explicit upstream HTTP rejection did not start a successful generation.
	if e.record.UpstreamStatus != nil && *e.record.UpstreamStatus >= 400 {
		state = "settled"
	}
	for _, r := range entries {
		if r.State != "active" {
			continue
		}
		if err := q.AddTokenBudgetUsage(ctx, db.AddTokenBudgetUsageParams{BudgetID: r.BudgetID, WindowStart: r.WindowStart, Tokens: tokens}); err != nil {
			return err
		}
		if err := q.SetTokenBudgetEntry(ctx, db.SetTokenBudgetEntryParams{RequestID: r.RequestID, BudgetID: r.BudgetID, State: state, Tokens: tokens}); err != nil {
			return err
		}
	}
	return pruneBudgets(ctx, q, e.service.now())
}
func pruneBudgets(ctx context.Context, q *db.Queries, now time.Time) error {
	before := now.AddDate(0, 0, -90).Unix()
	if err := q.PruneTokenBudgetEntries(ctx, before); err != nil {
		return err
	}
	return q.PruneTokenBudgetUsage(ctx, before)
}
func (s *Service) ResolveBudget(ctx context.Context, user int64, requestID string, tokens int64) error {
	if user <= 1 || !safeRequestID.MatchString(requestID) || tokens < 0 || tokens > 2_000_000_000 {
		return accounts.ErrInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.recoverBudgets(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	entries, err := q.ListTokenBudgetEntries(ctx, db.ListTokenBudgetEntriesParams{RequestID: requestID, UserID: user})
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return ErrBudgetSettlement
	}
	changed := false
	for _, r := range entries {
		if r.State == "settled" && r.Manual == 1 && r.Tokens == tokens {
			continue
		}
		if r.State != "pending" || tokens < r.Tokens {
			return ErrBudgetSettlement
		}
		if err := q.AddTokenBudgetUsage(ctx, db.AddTokenBudgetUsageParams{BudgetID: r.BudgetID, WindowStart: r.WindowStart, Tokens: tokens - r.Tokens}); err != nil {
			return err
		}
		if err := q.SetTokenBudgetEntry(ctx, db.SetTokenBudgetEntryParams{RequestID: r.RequestID, BudgetID: r.BudgetID, State: "settled", Tokens: tokens, Manual: 1}); err != nil {
			return err
		}
		changed = true
	}
	if changed {
		if err := audit.Record(ctx, q, "member.budget_settle", "member", audit.ID(user)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
