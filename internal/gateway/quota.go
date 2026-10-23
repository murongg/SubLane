package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/upstream"
)

var ErrQuotaExhausted = errors.New("quota_exhausted")

type QuotaError struct{ RetryAfter int64 }

func (e *QuotaError) Error() string { return ErrQuotaExhausted.Error() }
func (e *QuotaError) Unwrap() error { return ErrQuotaExhausted }

type quotaDecision struct {
	State   string
	RetryAt int64
	Weight  int
}

func quotaStatus(value upstream.Usage, now time.Time) quotaDecision {
	result := quotaDecision{State: "unknown"}
	if value.UpdatedAt <= 0 {
		return result
	}
	state := snapshotState(&usageEntry{snapshot: &value}, now)
	if state.Stale {
		result.State = "stale"
		return result
	}
	// Additional named limits are not a documented mapping to model IDs. Only the main
	// subscription limit can exclude the whole account; partial/absent data stays unknown.
	for _, limit := range value.Limits {
		if limit.Name != "" {
			continue
		}
		blocked := (limit.Allowed != nil && !*limit.Allowed) || (limit.LimitReached != nil && *limit.LimitReached)
		known := (limit.Allowed != nil && *limit.Allowed) || (limit.LimitReached != nil && !*limit.LimitReached)
		for _, window := range limit.Windows {
			if window.ResetAt != nil && *window.ResetAt <= now.Unix() {
				return quotaDecision{State: "stale"}
			}
			if window.UsedPercent != nil {
				known = true
				blocked = blocked || *window.UsedPercent >= 100
			}
		}
		if blocked {
			return quotaDecision{State: "exhausted", RetryAt: state.ExpiresAt}
		}
		if known {
			result.State = "available"
			remaining := 100.0
			for _, window := range limit.Windows {
				if window.UsedPercent == nil || math.IsNaN(*window.UsedPercent) || math.IsInf(*window.UsedPercent, 0) || *window.UsedPercent < 0 || *window.UsedPercent >= 100 {
					remaining = 0
					break
				}
				remaining = min(remaining, 100-*window.UsedPercent)
			}
			if len(limit.Windows) > 0 {
				result.Weight = int(math.Ceil(remaining))
			}
		}
	}
	return result
}

func savedUsage(row db.GetAccountUsageRow) *upstream.Usage {
	return decodeUsage(row.Snapshot, row.UpdatedAt)
}

func decodeUsage(raw []byte, updatedAt *int64) *upstream.Usage {
	var value upstream.Usage
	// A complete Antigravity snapshot can contain 512 limits, including hidden models.
	if updatedAt == nil || len(raw) > 256<<10 || json.Unmarshal(raw, &value) != nil || value.Limits == nil || value.UpdatedAt != *updatedAt || value.UpdatedAt <= 0 {
		return nil
	}
	return &value
}

func (s *Service) accountQuota(ctx context.Context, q *db.Queries, account accounts.Account, model string) (quotaDecision, error) {
	if !quotaRoutingSupported(account.Provider) {
		return quotaDecision{State: "unsupported"}, nil
	}
	row, err := q.GetAccountUsage(ctx, account.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return quotaDecision{State: "unknown"}, nil
	}
	if err != nil {
		return quotaDecision{}, err
	}
	return s.snapshotQuota(account.Provider, model, row.Snapshot, row.UpdatedAt), nil
}

func (s *Service) snapshotQuota(provider, model string, raw []byte, updatedAt *int64) quotaDecision {
	if !quotaRoutingSupported(provider) {
		return quotaDecision{State: "unsupported"}
	}
	if value := decodeUsage(raw, updatedAt); value != nil {
		return quotaStatus(quotaForModel(*value, provider, model), s.now())
	}
	return quotaDecision{State: "unknown"}
}

func quotaForModel(value upstream.Usage, provider, model string) upstream.Usage {
	if provider != "antigravity" {
		return value
	}
	// Exact model matching keeps one Antigravity limit from blocking unrelated models.
	id := upstream.CatalogModelID(model)
	for _, limit := range value.Limits {
		if id != "" && limit.Name == id {
			limit.Name = ""
			value.Limits = []upstream.UsageLimit{limit}
			return value
		}
	}
	value.Limits = nil
	return value
}
func (s *Service) quotaAdmission(ctx context.Context, q *db.Queries, account accounts.Account, model string) error {
	decision, err := s.accountQuota(ctx, q, account, model)
	if err != nil {
		return err
	}
	if decision.State == "exhausted" {
		return &QuotaError{RetryAfter: max(1, decision.RetryAt-s.now().Unix())}
	}
	return nil
}

// Traffic warms the existing shared cache without awaiting provider IO. Reading quota
// here (before selection opens its SQLite transaction) avoids a cache/DB lock inversion.
func (s *Service) warmUsage(ctx context.Context, userID, groupID int64, provider string) error {
	if provider != "" && !quotaRoutingSupported(provider) {
		return nil
	}
	if _, err := poolAccounts(ctx, s.queries, userID, groupID); err != nil {
		return err
	}
	rows, err := s.queries.ListGroupUsage(ctx, groupID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if s.accounts.ProviderEnabled(row.Provider) && quotaRoutingSupported(row.Provider) && (provider == "" || row.Provider == provider) {
			c := s.usage
			c.mu.Lock()
			now := c.now()
			e, err := c.load(row.ID, db.GetAccountUsageRow{Snapshot: row.Snapshot, UpdatedAt: row.UpdatedAt, Revision: row.Revision}, now)
			refresh := false
			if err == nil {
				refresh = e.flight == nil && !now.Before(e.retryAt) && (e.snapshot == nil || snapshotState(e, now).Stale)
			}
			c.mu.Unlock()
			if !refresh {
				continue
			}
			if _, err := s.usageSnapshot(ctx, row.ID, false, false); err != nil && ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
	return nil
}

func quotaRoutingSupported(provider string) bool {
	return provider == "codex" || provider == "claude" || provider == "antigravity"
}
