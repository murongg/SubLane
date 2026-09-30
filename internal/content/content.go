// Package content owns encrypted workspace rules and bounded, local request checks.
package content

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
)

var (
	ErrInput       = errors.New("invalid_content_rules")
	ErrConflict    = errors.New("content_rules_changed")
	ErrBlocked     = errors.New("content_policy_blocked")
	ErrUnavailable = errors.New("content_check_unavailable")
)

const MaxRules = 50
const MaxPattern = 1024
const MaxSample = 65536
const MaxScanBody = 8 << 20

type Rule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
}
type RuleInput struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Kind    string  `json:"kind"`
	Pattern *string `json:"pattern,omitempty"`
	Enabled bool    `json:"enabled"`
}
type State struct {
	Mode     string `json:"mode"`
	Revision int64  `json:"revision"`
	Rules    []Rule `json:"rules"`
}
type Input struct {
	Mode     string      `json:"mode"`
	Revision int64       `json:"revision"`
	Rules    []RuleInput `json:"rules"`
}
type Result struct {
	Mode            string   `json:"mode"`
	Revision        int64    `json:"revision"`
	RuleIDs         []string `json:"rule_ids"`
	CheckFailed     bool     `json:"check_failed"`
	MetadataMatched bool     `json:"-"`
}
type savedRule struct {
	Rule
	Pattern string `json:"pattern"`
}
type config struct {
	Mode     string      `json:"mode"`
	Revision int64       `json:"revision"`
	Rules    []savedRule `json:"rules"`
}
type matcher struct {
	rule       savedRule
	expression *regexp.Regexp
}
type policy struct {
	config   config
	matchers []matcher
}
type Service struct {
	connection *sql.DB
	queries    *db.Queries
	vault      *vault.Vault
	tenantID   int64
	mu         sync.Mutex
	encrypted  []byte
	cached     *policy
}

func New(connection *sql.DB, cipher *vault.Vault, tenantID int64) *Service {
	return &Service{connection: connection, queries: db.New(connection), vault: cipher, tenantID: tenantID}
}

func compile(c config) (*policy, error) {
	if (c.Mode != "off" && c.Mode != "observe" && c.Mode != "block") || c.Revision < 0 || len(c.Rules) > MaxRules {
		return nil, ErrInput
	}
	p := &policy{config: c}
	seen := map[string]bool{}
	for _, r := range c.Rules {
		if !validID(r.ID) || seen[r.ID] || len(strings.TrimSpace(r.Name)) == 0 || len(r.Name) > 64 || strings.ContainsAny(r.Name, "\r\n") {
			return nil, ErrInput
		}
		seen[r.ID] = true
		m, err := compileRule(r)
		if err != nil {
			return nil, err
		}
		p.matchers = append(p.matchers, m)
	}
	return p, nil
}
func validID(id string) bool {
	if !strings.HasPrefix(id, "rule_") || len(id) > 64 || len(id) < 6 {
		return false
	}
	for _, c := range id[5:] {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
func compileRule(r savedRule) (matcher, error) {
	m := matcher{rule: r}
	if len(r.Pattern) == 0 || len(r.Pattern) > MaxPattern {
		return m, ErrInput
	}
	switch r.Kind {
	case "text":
	case "regex":
		expression, err := regexp.Compile(r.Pattern)
		// Empty matches would flag every request, including requests with no secret.
		if err != nil || expression.MatchString("") {
			return m, ErrInput
		}
		m.expression = expression
	default:
		return m, ErrInput
	}
	return m, nil
}
func (m matcher) matches(text string) bool {
	if m.expression != nil {
		return m.expression.MatchString(text)
	}
	return strings.Contains(text, m.rule.Pattern)
}
func (p *policy) state() State {
	s := State{Mode: p.config.Mode, Revision: p.config.Revision, Rules: []Rule{}}
	for _, r := range p.config.Rules {
		s.Rules = append(s.Rules, r.Rule)
	}
	return s
}
func (s *Service) decode(encrypted []byte) (*policy, error) {
	if len(encrypted) == 0 {
		return compile(config{Mode: "off", Rules: []savedRule{}})
	}
	if s.vault == nil || len(encrypted) > 131072 {
		return nil, ErrUnavailable
	}
	plain, err := s.vault.OpenContent(s.tenantID, encrypted)
	if err != nil {
		return nil, ErrUnavailable
	}
	var c config
	if json.Unmarshal(plain, &c) != nil {
		return nil, ErrUnavailable
	}
	p, err := compile(c)
	if err != nil {
		return nil, ErrUnavailable
	}
	return p, nil
}
func (s *Service) loadLocked(ctx context.Context) (*policy, error) {
	encrypted, err := s.queries.GetContentConfig(ctx, s.tenantID)
	if err != nil {
		return nil, ErrUnavailable
	}
	if s.cached != nil && bytes.Equal(encrypted, s.encrypted) {
		return s.cached, nil
	}
	p, err := s.decode(encrypted)
	if err != nil {
		return nil, err
	}
	s.cached = p
	s.encrypted = bytes.Clone(encrypted)
	return p, nil
}
func (s *Service) load(ctx context.Context) (*policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(ctx)
}
func (s *Service) State(ctx context.Context) (State, error) {
	p, err := s.load(ctx)
	if err != nil {
		return State{}, err
	}
	return p.state(), nil
}
func (s *Service) Update(ctx context.Context, input Input) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prior, err := s.loadLocked(ctx)
	if err != nil {
		return State{}, err
	}
	if input.Revision != prior.config.Revision {
		return State{}, ErrConflict
	}
	c := config{Mode: input.Mode, Revision: input.Revision + 1, Rules: []savedRule{}}
	for _, r := range input.Rules {
		saved := savedRule{Rule: Rule{ID: r.ID, Name: strings.TrimSpace(r.Name), Kind: r.Kind, Enabled: r.Enabled}}
		if r.ID == "" {
			saved.ID = "rule_" + rand.Text()
		} else {
			found := false
			for _, old := range prior.config.Rules {
				if old.ID == r.ID {
					saved.Pattern = old.Pattern
					found = true
					break
				}
			}
			if !found {
				return State{}, ErrInput
			}
		}
		if r.Pattern != nil {
			saved.Pattern = *r.Pattern
		}
		c.Rules = append(c.Rules, saved)
	}
	next, err := compile(c)
	if err != nil {
		return State{}, err
	}
	plain, err := json.Marshal(c)
	if err != nil || s.vault == nil {
		return State{}, ErrUnavailable
	}
	encrypted, err := s.vault.SealContent(s.tenantID, plain)
	if err != nil || len(encrypted) > 131072 {
		return State{}, ErrUnavailable
	}
	tx, err := s.connection.BeginTx(ctx, nil)
	if err != nil {
		return State{}, ErrUnavailable
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	changed, err := q.SaveContentConfig(ctx, db.SaveContentConfigParams{Config: encrypted, TenantID: s.tenantID, Previous: s.encrypted})
	if err != nil {
		return State{}, ErrUnavailable
	}
	if changed != 1 {
		return State{}, ErrConflict
	}
	if audit.Record(ctx, q, "settings.update", "settings", "content") != nil {
		return State{}, ErrUnavailable
	}
	if tx.Commit() != nil {
		return State{}, ErrUnavailable
	}
	// Publish only after the encrypted policy and its management audit commit together.
	s.encrypted = bytes.Clone(encrypted)
	s.cached = next
	return next.state(), nil
}
func (s *Service) Test(ctx context.Context, input RuleInput, sample string) (bool, error) {
	if len(sample) > MaxSample {
		return false, ErrInput
	}
	r := savedRule{Rule: Rule{Kind: input.Kind}}
	if input.Pattern != nil {
		r.Pattern = *input.Pattern
	} else {
		p, err := s.load(ctx)
		if err != nil {
			return false, err
		}
		for _, old := range p.config.Rules {
			if old.ID == input.ID {
				r.Pattern = old.Pattern
				break
			}
		}
	}
	m, err := compileRule(r)
	if err != nil {
		return false, err
	}
	return m.matches(sample), nil
}
func (s *Service) Check(ctx context.Context, raw []byte, limit int64, metadata ...string) (Result, error) {
	p, err := s.load(ctx)
	if err != nil {
		return Result{RuleIDs: []string{}, CheckFailed: true}, ErrUnavailable
	}
	result := Result{Mode: p.config.Mode, Revision: p.config.Revision, RuleIDs: []string{}}
	if result.Mode == "off" {
		result.Mode = ""
		return result, nil
	}
	for _, text := range metadata {
		for _, m := range p.matchers {
			if m.rule.Enabled && m.matches(text) {
				result.MetadataMatched = true
				break
			}
		}
	}
	failed := func() (Result, error) {
		result.CheckFailed = true
		if result.Mode == "block" {
			return result, ErrUnavailable
		}
		return result, nil
	}
	// Enabled checks never approve a truncated prefix of a larger request.
	if limit <= 0 || int64(len(raw)) > min(limit, MaxScanBody) {
		return failed()
	}
	matched := make([]bool, len(p.matchers))
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	depth, tokens := 0, 0
	complete := false
	// Token scanning preserves duplicate fields: provider parsers may choose a different occurrence.
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if !complete || depth != 0 {
				return failed()
			}
			break
		}
		if err != nil || complete || ctx.Err() != nil {
			return failed()
		}
		tokens++
		if delimiter, ok := token.(json.Delim); ok {
			if delimiter == '{' || delimiter == '[' {
				depth++
			} else {
				depth--
			}
			if depth < 0 || depth > 64 {
				return failed()
			}
		}
		if text, ok := token.(string); ok {
			for i, m := range p.matchers {
				if ctx.Err() != nil {
					return failed()
				}
				if m.rule.Enabled && !matched[i] && m.matches(text) {
					matched[i] = true
				}
			}
		}
		complete = tokens > 0 && depth == 0
	}
	for i, m := range p.matchers {
		if matched[i] {
			result.RuleIDs = append(result.RuleIDs, m.rule.ID)
		}
	}
	if result.Mode == "block" && len(result.RuleIDs) > 0 {
		return result, ErrBlocked
	}
	return result, nil
}
func (s *Service) Verify(ctx context.Context) error {
	rows, err := s.queries.ListContentConfigs(ctx)
	if err != nil {
		return ErrUnavailable
	}
	for _, row := range rows {
		if _, err = New(s.connection, s.vault, row.ID).decode(row.ContentConfig); err != nil {
			return ErrUnavailable
		}
	}
	return nil
}
