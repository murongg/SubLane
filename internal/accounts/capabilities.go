package accounts

import (
	"regexp"
	"slices"
)

const MaxModelTokens = 1_000_000_000

var reasoningLevel = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,15}$`)

// Nil fields mean unreported, including booleans: unknown must never become false.
type ModelCapabilities struct {
	ContextWindow            *int64   `json:"context_window,omitempty"`
	MaxOutputTokens          *int64   `json:"max_output_tokens,omitempty"`
	SupportedReasoningLevels []string `json:"supported_reasoning_levels,omitempty"`
	SupportsImages           *bool    `json:"supports_images,omitempty"`
	SupportsTools            *bool    `json:"supports_tools,omitempty"`
}

func (c ModelCapabilities) Valid() bool {
	for _, limit := range []*int64{c.ContextWindow, c.MaxOutputTokens} {
		if limit != nil && (*limit <= 0 || *limit > MaxModelTokens) {
			return false
		}
	}
	if len(c.SupportedReasoningLevels) > 8 {
		return false
	}
	for _, level := range c.SupportedReasoningLevels {
		if !reasoningLevel.MatchString(level) {
			return false
		}
	}
	return true
}

func (c ModelCapabilities) Empty() bool {
	return c.ContextWindow == nil && c.MaxOutputTokens == nil && len(c.SupportedReasoningLevels) == 0 && c.SupportsImages == nil && c.SupportsTools == nil
}

func normalizedCapabilities(models []string, capabilities map[string]ModelCapabilities) (map[string]ModelCapabilities, error) {
	if len(capabilities) > len(models) {
		return nil, ErrInput
	}
	result := make(map[string]ModelCapabilities, len(capabilities))
	for id, capability := range capabilities {
		if !slices.Contains(models, id) || !capability.Valid() {
			return nil, ErrInput
		}
		if !capability.Empty() {
			result[id] = capability
		}
	}
	return result, nil
}
