// Package allocations owns exclusive pool schemes and their accounting.
package allocations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/pricing"
	"github.com/murongg/SubLane/internal/storage/db"
)

var (
	ErrInput        = errors.New("invalid_allocation_input")
	ErrNotFound     = errors.New("allocation_not_found")
	ErrPoolConflict = errors.New("allocation_pool_conflict")
	ErrUnavailable  = errors.New("allocation_unavailable")
	ErrQuota        = errors.New("allocation_exhausted")
	ErrPending      = errors.New("allocation_pending")
	ErrSync         = errors.New("allocation_syncing")
	ErrUnpriced     = errors.New("allocation_model_unpriced")
	ErrSnapshot     = errors.New("allocation_snapshot_required")
	ErrSettlement   = errors.New("invalid_allocation_settlement")
)

type Share struct {
	UserID int64 `json:"user_id"`
	Limit  int64 `json:"limit"`
}

// Rates are micro-USD per million tokens. Ratio mode uses the same units only as relative weights.
type Rate struct {
	Model  string `json:"model"`
	Input  int64  `json:"input"`
	Cached int64  `json:"cached"`
	Output int64  `json:"output"`
}
type Config struct {
	Mode            string  `json:"mode"`
	Period          string  `json:"period"`
	Members         []Share `json:"members"`
	Rates           []Rate  `json:"rates"`
	AllowIdleBorrow bool    `json:"allow_idle_borrow,omitempty"`
}
type SchemeInput struct {
	Name      string `json:"name"`
	GroupID   int64  `json:"group_id"`
	Enabled   bool   `json:"enabled"`
	StartNext bool   `json:"start_next"`
	Config    Config `json:"config"`
}
type Revision struct {
	ID          int64  `json:"id"`
	EffectiveAt int64  `json:"effective_at"`
	Config      Config `json:"config"`
}
type Scheme struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	Revision
	Next *Revision `json:"next"`
}
type Service struct {
	conn     *sql.DB
	tenantID int64
	now      func() time.Time
	pricing  *pricing.Service
}

func New(conn *sql.DB) *Service { return NewForTenant(conn, 1) }
func NewForTenant(conn *sql.DB, tenantID int64) *Service {
	return &Service{conn: conn, tenantID: tenantID, now: time.Now}
}
func NewWithPricing(conn *sql.DB, catalog *pricing.Service) *Service {
	return NewForTenantWithPricing(conn, 1, catalog)
}
func NewForTenantWithPricing(conn *sql.DB, tenantID int64, catalog *pricing.Service) *Service {
	return &Service{conn: conn, tenantID: tenantID, now: time.Now, pricing: catalog}
}
func validName(v string) bool {
	return v == strings.TrimSpace(v) && utf8.ValidString(v) && utf8.RuneCountInString(v) > 0 && utf8.RuneCountInString(v) <= 64 && strings.IndexFunc(v, unicode.IsControl) < 0
}
func bit(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
func normalize(c *Config) error {
	if (c.Mode != "tokens" && c.Mode != "amount" && c.Mode != "ratio") || len(c.Members) == 0 || len(c.Members) > 100 || len(c.Rates) > 128 {
		return ErrInput
	}
	if c.Mode == "ratio" {
		if c.Period != "upstream" {
			return ErrInput
		}
	} else if c.Period != "day" && c.Period != "month" {
		return ErrInput
	}
	seen := map[int64]bool{}
	var total int64
	for _, m := range c.Members {
		if m.UserID <= 0 || seen[m.UserID] || m.Limit <= 0 || m.Limit > 1_000_000_000_000 {
			return ErrInput
		}
		seen[m.UserID] = true
		total += m.Limit
	}
	if c.Mode == "ratio" && total > 10000 {
		return ErrInput
	}
	if c.Mode == "amount" && len(c.Rates) == 0 {
		return ErrInput
	}
	models := map[string]bool{}
	for i, r := range c.Rates {
		_, model := groups.SplitModel(r.Model)
		if model == "" || len(model) > 128 || models[model] || strings.ContainsAny(model, " \t\n*") {
			return ErrInput
		}
		models[model] = true
		c.Rates[i].Model = model
		if r.Input <= 0 || r.Cached < 0 || r.Output <= 0 || r.Input > 1_000_000_000 || r.Cached > 1_000_000_000 || r.Output > 1_000_000_000 {
			return ErrInput
		}
	}
	if c.Rates == nil {
		c.Rates = []Rate{}
	}
	slices.SortFunc(c.Members, func(a, b Share) int {
		if a.UserID < b.UserID {
			return -1
		}
		if a.UserID > b.UserID {
			return 1
		}
		return 0
	})
	return nil
}
func Current(ctx context.Context, q *db.Queries, id, now int64) (Revision, error) {
	r, err := q.CurrentAllocationRevision(ctx, db.CurrentAllocationRevisionParams{SchemeID: id, EffectiveAt: now})
	if errors.Is(err, sql.ErrNoRows) {
		return Revision{}, ErrUnavailable
	}
	if err != nil {
		return Revision{}, err
	}
	return decodeRevision(r.ID, r.EffectiveAt, r.Config)
}
func decodeRevision(id, at int64, raw string) (Revision, error) {
	r := Revision{ID: id, EffectiveAt: at}
	if err := json.Unmarshal([]byte(raw), &r.Config); err != nil {
		return r, err
	}
	return r, nil
}
func Window(now time.Time, period string) (int64, int64) {
	v := now.UTC()
	start := time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, time.UTC)
	if period == "month" {
		start = time.Date(v.Year(), v.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start.Unix(), start.AddDate(0, 1, 0).Unix()
	}
	return start.Unix(), start.AddDate(0, 0, 1).Unix()
}
func (s *Service) Schemes(ctx context.Context) ([]Scheme, error) {
	q := db.New(s.conn)
	rows, err := q.ListAllocationSchemes(ctx, s.tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]Scheme, 0, len(rows))
	for _, r := range rows {
		v, err := readScheme(ctx, q, s.tenantID, r.ID, s.now().Unix())
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
func readScheme(ctx context.Context, q *db.Queries, tenantID, id, now int64) (Scheme, error) {
	r, err := q.GetTenantAllocationScheme(ctx, db.GetTenantAllocationSchemeParams{ID: id, TenantID: tenantID})
	if err != nil {
		return Scheme{}, ErrNotFound
	}
	out := Scheme{ID: r.ID, Name: r.Name, GroupID: r.GroupID, GroupName: r.GroupName, Enabled: r.Enabled == 1, CreatedAt: r.CreatedAt}
	out.Revision, err = Current(ctx, q, id, now)
	if err != nil && !errors.Is(err, ErrUnavailable) {
		return out, err
	}
	next, e := q.NextAllocationRevision(ctx, db.NextAllocationRevisionParams{SchemeID: id, EffectiveAt: now})
	if e == nil {
		rev, e := decodeRevision(next.ID, next.EffectiveAt, next.Config)
		if e != nil {
			return out, e
		}
		out.Next = &rev
		if out.Revision.ID == 0 {
			out.Revision = rev
		}
	} else if !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	return out, nil
}
func (s *Service) SaveScheme(ctx context.Context, id int64, in SchemeInput) (Scheme, error) {
	if id < 0 || !validName(in.Name) || in.GroupID <= 0 {
		return Scheme{}, ErrInput
	}
	if in.Config.Mode == "ratio" {
		var total int64
		for _, member := range in.Config.Members {
			total += member.Limit
		}
		if total > 10000 {
			return Scheme{}, ErrInput
		}
	} else if in.Config.AllowIdleBorrow {
		return Scheme{}, ErrInput
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return Scheme{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	now := s.now().Unix()
	if _, err := q.GetTenantGroup(ctx, db.GetTenantGroupParams{ID: in.GroupID, TenantID: s.tenantID}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Scheme{}, ErrInput
		}
		return Scheme{}, err
	}
	for _, m := range in.Config.Members {
		allowed, err := q.CanUseGroup(ctx, db.CanUseGroupParams{UserID: m.UserID, GroupID: in.GroupID})
		if err != nil {
			return Scheme{}, err
		}
		if !allowed {
			return Scheme{}, ErrInput
		}
	}
	accounts, err := q.AllocationPoolAccounts(ctx, in.GroupID)
	if err != nil {
		return Scheme{}, err
	}
	if len(accounts) == 0 {
		return Scheme{}, ErrInput
	}
	for _, a := range accounts {
		if a.Shared > 0 {
			return Scheme{}, ErrPoolConflict
		}
		if in.Config.Mode == "ratio" && a.Provider != "codex" {
			return Scheme{}, ErrInput
		}
	}
	if err = s.applyPrices(ctx, q, in.GroupID, &in.Config); err != nil {
		return Scheme{}, err
	}
	if normalize(&in.Config) != nil {
		return Scheme{}, ErrInput
	}
	effective := now
	if id == 0 {
		if _, err = q.GetPoolAllocation(ctx, in.GroupID); err == nil {
			return Scheme{}, ErrPoolConflict
		} else if !errors.Is(err, sql.ErrNoRows) {
			return Scheme{}, err
		}
		rows, e := q.ListAllocationSchemes(ctx, s.tenantID)
		if e != nil {
			return Scheme{}, e
		}
		if len(rows) >= 32 {
			return Scheme{}, ErrInput
		}
		id, err = q.CreateAllocationScheme(ctx, db.CreateAllocationSchemeParams{Name: in.Name, GroupID: in.GroupID, Enabled: bit(in.Enabled), CreatedAt: now})
		if err != nil {
			return Scheme{}, err
		}
		if in.StartNext {
			effective, err = nextEffective(ctx, q, in.GroupID, in.Config, now)
			if err != nil {
				return Scheme{}, err
			}
		}
	} else {
		old, err := q.GetTenantAllocationScheme(ctx, db.GetTenantAllocationSchemeParams{ID: id, TenantID: s.tenantID})
		if err != nil {
			return Scheme{}, ErrNotFound
		}
		if old.GroupID != in.GroupID {
			return Scheme{}, ErrInput
		}
		if err = q.UpdateAllocationScheme(ctx, db.UpdateAllocationSchemeParams{ID: id, Name: in.Name, Enabled: bit(in.Enabled)}); err != nil {
			return Scheme{}, err
		}
		current, err := Current(ctx, q, id, now)
		if err == nil {
			effective, err = nextEffective(ctx, q, in.GroupID, current.Config, now)
			if err != nil {
				return Scheme{}, err
			}
		} else if errors.Is(err, ErrUnavailable) {
			next, e := q.NextAllocationRevision(ctx, db.NextAllocationRevisionParams{SchemeID: id, EffectiveAt: now})
			if e != nil {
				return Scheme{}, e
			}
			effective = next.EffectiveAt
		} else {
			return Scheme{}, err
		}
		if err = q.DeleteNextAllocationRevisions(ctx, db.DeleteNextAllocationRevisionsParams{SchemeID: id, EffectiveAt: now}); err != nil {
			return Scheme{}, err
		}
	}
	raw, err := json.Marshal(in.Config)
	if err != nil {
		return Scheme{}, err
	}
	if _, err = q.SaveAllocationRevision(ctx, db.SaveAllocationRevisionParams{SchemeID: id, EffectiveAt: effective, Config: string(raw)}); err != nil {
		return Scheme{}, err
	}
	if err = audit.Record(ctx, q, "allocation.save", "allocation", audit.ID(id)); err != nil {
		return Scheme{}, err
	}
	out, err := readScheme(ctx, q, s.tenantID, id, now)
	if err != nil {
		return Scheme{}, err
	}
	return out, tx.Commit()
}

// applyPrices resolves omitted or zero model rates once, before the config is
// persisted into a revision. Existing non-zero values remain explicit overrides.
func (s *Service) applyPrices(ctx context.Context, q *db.Queries, groupID int64, config *Config) error {
	if config.Mode == "tokens" || s.pricing == nil {
		if config.Mode == "ratio" && len(config.Rates) == 0 {
			return ErrUnpriced
		}
		return nil
	}
	if config.Mode == "ratio" && len(config.Rates) == 0 && q != nil {
		rows, err := q.ListGroupCatalogs(ctx, groupID)
		if err != nil {
			return err
		}
		models := map[string]bool{}
		for _, row := range rows {
			catalog, err := accounts.DecodeCatalog(row.ModelsSnapshot, row.ModelsRevision)
			if err != nil {
				continue
			}
			for _, model := range catalog.Models {
				_, native := groups.SplitModel(model)
				models[native] = true
			}
		}
		if len(models) == 0 {
			return ErrUnpriced
		}
		ids := make([]string, 0, len(models))
		for model := range models {
			ids = append(ids, model)
		}
		sort.Strings(ids)
		for _, model := range ids {
			price, ok := s.pricing.Lookup(model)
			if !ok {
				return ErrUnpriced
			}
			config.Rates = append(config.Rates, Rate{Model: model, Input: price.Input, Cached: price.Cached, Output: price.Output})
		}
		return nil
	}
	for i := range config.Rates {
		model := config.Rates[i].Model
		price, ok := s.pricing.Lookup(model)
		if !ok {
			continue
		}
		if config.Rates[i].Input == 0 {
			config.Rates[i].Input = price.Input
		}
		if config.Rates[i].Cached == 0 {
			config.Rates[i].Cached = price.Cached
		}
		if config.Rates[i].Output == 0 {
			config.Rates[i].Output = price.Output
		}
	}
	return nil
}
func Cost(r Rate, input, output, cached int64) (int64, error) {
	if input < 0 || output < 0 || cached < 0 || cached > input || input > 1_000_000_000 || output > 1_000_000_000 {
		return 0, ErrInput
	}
	// Round only after summing all token categories; even sub-micro charges remain nonzero.
	sum := new(big.Int)
	for _, v := range [][2]int64{{input - cached, r.Input}, {cached, r.Cached}, {output, r.Output}} {
		if v[1] < 0 {
			return 0, ErrInput
		}
		sum.Add(sum, new(big.Int).Mul(big.NewInt(v[0]), big.NewInt(v[1])))
	}
	sum.Add(sum, big.NewInt(999999))
	sum.Div(sum, big.NewInt(1000000))
	if !sum.IsInt64() {
		return 0, ErrInput
	}
	return sum.Int64(), nil
}

// Largest remainders conserve the exact observed upstream change, including one-point intervals.
func Distribute(points int64, weights []int64) ([]int64, error) {
	if points < 0 || points > 10000 || len(weights) == 0 {
		return nil, ErrInput
	}
	total := new(big.Int)
	for _, w := range weights {
		if w < 0 {
			return nil, ErrInput
		}
		total.Add(total, big.NewInt(w))
	}
	if total.Sign() == 0 {
		return nil, ErrPending
	}
	out := make([]int64, len(weights))
	remainders := make([]*big.Int, len(weights))
	var assigned int64
	for i, w := range weights {
		product := new(big.Int).Mul(big.NewInt(points), big.NewInt(w))
		value, rem := new(big.Int), new(big.Int)
		value.QuoRem(product, total, rem)
		out[i] = value.Int64()
		assigned += out[i]
		remainders[i] = rem
	}
	for assigned < points {
		best := 0
		for i := range remainders {
			if remainders[i].Cmp(remainders[best]) > 0 {
				best = i
			}
		}
		out[best]++
		remainders[best].SetInt64(-1)
		assigned++
	}
	return out, nil
}

// Access toggles never depend on upstream availability or rewrite the policy revision.
func (s *Service) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	row, err := q.GetTenantAllocationScheme(ctx, db.GetTenantAllocationSchemeParams{ID: id, TenantID: s.tenantID})
	if err != nil {
		return ErrNotFound
	}
	if err = q.UpdateAllocationScheme(ctx, db.UpdateAllocationSchemeParams{ID: id, Name: row.Name, Enabled: bit(enabled)}); err != nil {
		return err
	}
	if err = audit.Record(ctx, q, "allocation.save", "allocation", audit.ID(id)); err != nil {
		return err
	}
	return tx.Commit()
}
