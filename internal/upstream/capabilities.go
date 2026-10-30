package upstream

import (
	"encoding/json"
	"slices"
	"sort"

	"github.com/murongg/SubLane/internal/accounts"
)

// Optional metadata is independent of model membership: malformed details remain unknown.
func discoveryCapabilities(raw json.RawMessage, provider string) accounts.ModelCapabilities {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return accounts.ModelCapabilities{}
	}
	field := func(path ...string) json.RawMessage {
		current := fields
		for index, key := range path {
			if index == len(path)-1 {
				return current[key]
			}
			var nested map[string]json.RawMessage
			if json.Unmarshal(current[key], &nested) != nil {
				return nil
			}
			current = nested
		}
		return nil
	}
	tokenLimit := func(path ...string) *int64 {
		var value int64
		if json.Unmarshal(field(path...), &value) != nil || value <= 0 || value > accounts.MaxModelTokens {
			return nil
		}
		return &value
	}
	boolean := func(path ...string) *bool {
		var value *bool
		if json.Unmarshal(field(path...), &value) != nil {
			return nil
		}
		return value
	}
	stringList := func(path ...string) []string {
		var value []string
		if json.Unmarshal(field(path...), &value) != nil || len(value) > 64 {
			return nil
		}
		for _, item := range value {
			if item == "" || len(item) > 64 {
				return nil
			}
		}
		return value
	}
	result := accounts.ModelCapabilities{ContextWindow: tokenLimit("context_window"), MaxOutputTokens: tokenLimit("max_output_tokens"), SupportsImages: boolean("supports_images"), SupportsTools: boolean("supports_tools")}
	levels := stringList("supported_reasoning_levels")
	if provider == "codex" && levels == nil {
		var options []struct {
			Effort string `json:"effort"`
		}
		if json.Unmarshal(field("supported_reasoning_levels"), &options) == nil {
			for _, option := range options {
				levels = append(levels, option.Effort)
			}
		}
	}
	if provider == "codex" {
		if result.SupportsImages == nil {
			if modalities := stringList("input_modalities"); len(modalities) > 0 && len(modalities) <= 8 {
				value := slices.Contains(modalities, "image")
				result.SupportsImages = &value
			}
		}
		if result.SupportsTools == nil {
			var shell string
			json.Unmarshal(field("shell_type"), &shell)
			if slices.Contains([]string{"default", "local", "shell_command", "unified_exec"}, shell) || len(stringList("experimental_supported_tools")) > 0 {
				value := true
				result.SupportsTools = &value
			}
		}
	}
	if provider == "claude" || provider == "anthropic" {
		if result.ContextWindow == nil {
			result.ContextWindow = tokenLimit("max_input_tokens")
		}
		if result.MaxOutputTokens == nil {
			result.MaxOutputTokens = tokenLimit("max_tokens")
		}
		if result.SupportsImages == nil {
			result.SupportsImages = boolean("capabilities", "image_input", "supported")
		}
		// No server tools does not prove client-defined tools are unsupported.
		if tools := boolean("capabilities", "server_tools", "supported"); result.SupportsTools == nil && tools != nil && *tools {
			result.SupportsTools = tools
		}
		if levels == nil {
			var efforts map[string]json.RawMessage
			if json.Unmarshal(field("capabilities", "effort"), &efforts) == nil {
				for level, raw := range efforts {
					var capability struct {
						Supported bool `json:"supported"`
					}
					if json.Unmarshal(raw, &capability) == nil && capability.Supported {
						levels = append(levels, level)
					}
				}
				sort.Strings(levels)
			}
		}
	}
	if provider == "gemini" {
		if result.ContextWindow == nil {
			result.ContextWindow = tokenLimit("inputTokenLimit")
		}
		if result.MaxOutputTokens == nil {
			result.MaxOutputTokens = tokenLimit("outputTokenLimit")
		}
	}
	if provider == "antigravity" {
		if result.ContextWindow == nil {
			result.ContextWindow = tokenLimit("maxTokens")
		}
		if result.MaxOutputTokens == nil {
			result.MaxOutputTokens = tokenLimit("maxOutputTokens")
		}
	}
	// OpenRouter-compatible catalogs explicitly describe modalities and supported parameters.
	if result.ContextWindow == nil {
		result.ContextWindow = tokenLimit("context_length")
	}
	if result.MaxOutputTokens == nil {
		result.MaxOutputTokens = tokenLimit("top_provider", "max_completion_tokens")
	}
	if result.SupportsImages == nil {
		if modalities := stringList("architecture", "input_modalities"); len(modalities) > 0 && len(modalities) <= 8 {
			value := slices.Contains(modalities, "image")
			result.SupportsImages = &value
		}
	}
	if result.SupportsTools == nil {
		if parameters := stringList("supported_parameters"); len(parameters) > 0 {
			value := slices.Contains(parameters, "tools")
			result.SupportsTools = &value
		}
	}
	if (accounts.ModelCapabilities{SupportedReasoningLevels: levels}).Valid() {
		result.SupportedReasoningLevels = levels
	}
	return result
}
