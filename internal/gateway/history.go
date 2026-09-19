package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/upstream"
)

type requestKey struct{}
type requestIdentity struct {
	KeyID     int64
	Transport string
}

func WithRequestIdentity(ctx context.Context, keyID int64, transport string) context.Context {
	if transport != "websocket" {
		transport = "http"
	}
	return context.WithValue(ctx, requestKey{}, requestIdentity{KeyID: keyID, Transport: transport})
}

var safeModel = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/()+-]{0,159}$`)

type RequestRecord struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	KeyID          int64  `json:"key_id"`
	GroupID        int64  `json:"group_id"`
	AccountID      string `json:"account_id"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Transport      string `json:"transport"`
	Operation      string `json:"operation"`
	StartedAt      int64  `json:"started_at"`
	DurationMs     int64  `json:"duration_ms"`
	Outcome        string `json:"outcome"`
	ErrorCode      string `json:"error_code"`
	UpstreamStatus *int64 `json:"upstream_status"`
	InputTokens    *int64 `json:"input_tokens"`
	OutputTokens   *int64 `json:"output_tokens"`
	CachedTokens   *int64 `json:"cached_tokens"`
	Username       string `json:"username"`
	KeyName        string `json:"key_name"`
	GroupName      string `json:"group_name"`
	AccountName    string `json:"account_name"`
}
type RequestPage struct {
	Requests   []RequestRecord `json:"requests"`
	NextCursor int64           `json:"next_cursor"`
}

func (s *Service) Requests(ctx context.Context, cursor int64, accountID, outcome string) (RequestPage, error) {
	return s.requests(ctx, 0, cursor, accountID, outcome)
}

func (s *Service) UserRequests(ctx context.Context, userID, cursor int64, outcome string) (RequestPage, error) {
	// Zero is reserved for the administrator query, never for an absent personal identity.
	if userID <= 0 {
		return RequestPage{}, accounts.ErrInput
	}
	return s.requests(ctx, userID, cursor, "", outcome)
}

func (s *Service) requests(ctx context.Context, userID, cursor int64, accountID, outcome string) (RequestPage, error) {
	page := RequestPage{Requests: []RequestRecord{}}
	if cursor < 0 || len(accountID) > 64 {
		return page, accounts.ErrInput
	}
	switch outcome {
	case "", "success", "incomplete", "error", "canceled", "rejected":
	default:
		return page, accounts.ErrInput
	}
	since := s.now().Add(-7 * 24 * time.Hour).Unix()
	if err := s.queries.PruneRequests(ctx, since); err != nil {
		return page, err
	}
	rows, err := s.queries.ListRequests(ctx, db.ListRequestsParams{UserID: userID, Cursor: cursor, AccountID: accountID, Outcome: outcome, Since: since})
	if err != nil {
		return page, err
	}
	for _, row := range rows {
		record := RequestRecord(row)
		if userID != 0 {
			// Subscription identities belong to the operator even when they served this user's request.
			record.AccountID, record.AccountName = "", ""
		}
		page.Requests = append(page.Requests, record)
	}
	if len(page.Requests) > 50 {
		page.Requests = page.Requests[:50]
		page.NextCursor = page.Requests[49].ID
	}
	return page, nil
}

type observation struct {
	service    *Service
	ctx        context.Context
	cancel     context.CancelFunc
	stopParent func() bool
	once       sync.Once
	started    time.Time
	record     db.RecordRequestParams
	revision   int64
	leased     bool
	sequence   int64
}

func (s *Service) begin(ctx context.Context, userID, groupID int64, kind Kind) (*observation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, context.Canceled
	}
	s.workers.Add(1)
	s.sequence++
	operation, cancel := context.WithCancel(ctx)
	identity, _ := ctx.Value(requestKey{}).(requestIdentity)
	if identity.Transport == "" {
		identity.Transport = "http"
	}
	name := "responses"
	if kind == Chat {
		name = "chat"
	} else if kind == Compact {
		name = "compact"
	}
	started := s.now()
	return &observation{sequence: s.sequence, service: s, ctx: operation, cancel: cancel, stopParent: context.AfterFunc(s.runContext, cancel), started: started, record: db.RecordRequestParams{UserID: userID, GroupID: groupID, KeyID: identity.KeyID, Transport: identity.Transport, Operation: name, StartedAt: started.Unix()}}, nil
}
func (e *observation) finish(outcome, code, penalty, retry string) {
	e.once.Do(func() {
		defer e.service.workers.Done()
		defer e.cancel()
		defer e.stopParent()
		e.record.Outcome = outcome
		e.record.ErrorCode = code
		e.record.DurationMs = max(0, time.Since(e.started).Milliseconds())
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		s := e.service
		s.mu.Lock()
		if e.leased {
			if state := s.health[e.record.AccountID]; state != nil {
				changed := false
				if state.revision == e.revision {
					if penalty != "" {
						state.Failures = min(16, state.Failures+1)
						state.lastFailureAt = s.now().UnixMilli()
						s.sequence++
						state.lastFailureSequence = s.sequence
						state.Reason = penalty
						changed = true
						delay := int64(0)
						if penalty == "rate_limited" {
							delay = retryDelay(retry, s.now())
						} else if state.Failures >= 3 {
							delay = min(300, int64(30)<<min(4, state.Failures-3))
						}
						if delay > 0 {
							state.CooldownUntil = max(state.CooldownUntil, s.now().Unix()+delay)
						}
						// Sequence order, rather than wall-clock milliseconds, protects against late successes in the same tick.
					} else if (outcome == "success" || outcome == "incomplete") && state.lastFailureSequence < e.sequence && state.CooldownUntil <= s.now().Unix() {
						changed = state.Failures != 0 || state.CooldownUntil != 0
						state.Failures = 0
						state.CooldownUntil = 0
						state.Reason = ""
						state.lastFailureAt = 0
						state.lastFailureSequence = 0
					}
					if changed {
						if err := s.persistRuntime(ctx, state); err != nil {
							slog.Error("Unable to persist account cooldown", "account_id", state.ID)
						}
					}
				}
				state.InFlight = max(0, state.InFlight-1)
			}
		}
		s.mu.Unlock()
		tx, err := s.db.BeginTx(ctx, nil)
		if err == nil {
			defer tx.Rollback()
			q := s.queries.WithTx(tx)
			err = q.RecordRequest(ctx, e.record)
			if err == nil {
				err = q.PruneRequests(ctx, s.now().Add(-7*24*time.Hour).Unix())
			}
			if err == nil {
				err = tx.Commit()
			}
		}
		if err != nil {
			slog.Error("Unable to persist request metadata")
		}
	})
}
func classify(ctx context.Context, err error) (outcome, code, penalty string) {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return "canceled", "client_disconnected", ""
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return "error", "timeout", "timeout"
	}
	switch {
	case errors.Is(err, ErrAccountBusy):
		return "rejected", "account_busy", ""
	case errors.Is(err, ErrAccountCooling):
		return "rejected", "account_cooling", ""
	case errors.Is(err, ErrNoAccount), errors.Is(err, ErrAffinityUnavailable), errors.Is(err, accounts.ErrDisabled), errors.Is(err, accounts.ErrNotFound):
		return "rejected", "account_unavailable", ""
	case errors.Is(err, groups.ErrUnavailable):
		return "rejected", "group_unavailable", ""
	case errors.Is(err, accounts.ErrReauthorize):
		return "error", "auth_required", ""
	case errors.Is(err, upstream.ErrInput), errors.Is(err, upstream.ErrContinuation):
		return "rejected", "invalid_request", ""
	case errors.Is(err, upstream.ErrInterrupted), errors.Is(err, upstream.ErrResponse):
		return "error", "stream_interrupted", "upstream_error"
	case errors.Is(err, accounts.ErrRefresh):
		return "error", "refresh_failed", "upstream_error"
	case errors.Is(err, upstream.ErrUpstream):
		return "error", "upstream_unavailable", "upstream_error"
	default:
		return "error", "request_failed", ""
	}
}
func (e *observation) fail(err error) {
	var rejected *upstream.UpstreamError
	if errors.As(err, &rejected) {
		status := int64(rejected.Status)
		e.record.UpstreamStatus = &status
		outcome, code, penalty := statusOutcome(rejected.Status)
		e.finish(outcome, code, penalty, rejected.RetryAfter)
		return
	}
	outcome, code, penalty := classify(e.ctx, err)
	e.finish(outcome, code, penalty, "")
}

func (e *observation) observe(raw []byte) {
	type usage struct {
		Input   *int64 `json:"input_tokens"`
		Output  *int64 `json:"output_tokens"`
		Details struct {
			Cached *int64 `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	}
	var value struct {
		Response struct {
			Usage *usage `json:"usage"`
		} `json:"response"`
		Usage *usage `json:"usage"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return
	}
	tokens := value.Response.Usage
	if tokens == nil {
		tokens = value.Usage
	}
	if tokens == nil {
		return
	}
	valid := func(n *int64) *int64 {
		if n == nil || *n < 0 || *n > 1_000_000_000 {
			return nil
		}
		return n
	}
	e.record.InputTokens = valid(tokens.Input)
	e.record.OutputTokens = valid(tokens.Output)
	e.record.CachedTokens = valid(tokens.Details.Cached)
}
