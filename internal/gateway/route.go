package gateway

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/groups"
)

type routeKey struct{}

// RouteExplanation contains only fixed codes and bounded counts, never candidate identities.
type RouteExplanation struct {
	Version        int             `json:"version"`
	Strategy       string          `json:"strategy"`
	Result         string          `json:"result"`
	Candidates     int             `json:"candidates"`
	ParentFallback bool            `json:"parent_fallback"`
	Exclusions     RouteExclusions `json:"exclusions"`
}

type RouteExclusions struct {
	Model       int `json:"model"`
	Policy      int `json:"policy"`
	Quota       int `json:"quota"`
	Cooldown    int `json:"cooldown"`
	Concurrency int `json:"concurrency"`
	Unavailable int `json:"unavailable"`
}

const maxRouteCount = 10000

func (r *RouteExclusions) reject(err error) {
	var count *int
	switch {
	case errors.Is(err, ErrModelUnavailable), errors.Is(err, ErrCatalogUnavailable), errors.Is(err, ErrModelConflict):
		count = &r.Model
	case errors.Is(err, ErrModelNotAllowed), errors.Is(err, ErrAffinityUnavailable), errors.Is(err, groups.ErrUnavailable):
		count = &r.Policy
	case errors.Is(err, ErrQuotaExhausted), errors.Is(err, allocations.ErrQuota), errors.Is(err, allocations.ErrPending), errors.Is(err, allocations.ErrRisk), errors.Is(err, allocations.ErrUnpriced), errors.Is(err, allocations.ErrUnavailable):
		count = &r.Quota
	case errors.Is(err, ErrAccountCooling), errors.Is(err, ErrModelCooling):
		count = &r.Cooldown
	case errors.Is(err, ErrAccountBusy), errors.Is(err, ErrBusy):
		count = &r.Concurrency
	case errors.Is(err, accounts.ErrNotFound), errors.Is(err, accounts.ErrProviderDisabled), errors.Is(err, accounts.ErrDisabled), errors.Is(err, accounts.ErrReauthorize):
		count = &r.Unavailable
	}
	if count != nil {
		*count = min(maxRouteCount, *count+1)
	}
}

func encodeRoute(value *RouteExplanation) string {
	if value == nil || value.Version != 1 {
		return "null"
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 1024 {
		return "null"
	}
	return string(raw)
}

func decodeRoute(raw string) *RouteExplanation {
	if len(raw) > 1024 || !json.Valid([]byte(raw)) {
		return nil
	}
	// Missing and null JSON fields decode as zero values, which could invent a decision.
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &fields) != nil || len(fields) != 6 {
		return nil
	}
	var exclusions map[string]json.RawMessage
	if json.Unmarshal(fields["exclusions"], &exclusions) != nil || len(exclusions) != 6 {
		return nil
	}
	for _, object := range []map[string]json.RawMessage{fields, exclusions} {
		for _, field := range object {
			if bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
				return nil
			}
		}
	}
	var value *RouteExplanation
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || value == nil || value.Version != 1 {
		return nil
	}
	switch value.Strategy {
	case "none", "affinity", "parent", "rotation", "quota_weighted":
	default:
		return nil
	}
	if value.Result != "selected" && value.Result != "rejected" {
		return nil
	}
	if value.Result == "selected" && (value.Strategy == "none" || value.Candidates == 0) {
		return nil
	}
	for _, count := range []int{value.Candidates, value.Exclusions.Model, value.Exclusions.Policy, value.Exclusions.Quota, value.Exclusions.Cooldown, value.Exclusions.Concurrency, value.Exclusions.Unavailable} {
		if count < 0 || count > maxRouteCount {
			return nil
		}
	}
	return value
}
