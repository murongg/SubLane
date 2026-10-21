package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/murongg/SubLane/internal/upstream"
)

// Untrusted metadata must agree before it can influence local account selection.
func reviewParentThread(input map[string]json.RawMessage, headers http.Header, model string, kind Kind) string {
	if kind != Responses || upstream.CatalogModelID(model) != "codex-auto-review" {
		return ""
	}
	for _, key := range []string{"X-OpenAI-Subagent", "X-Codex-Parent-Thread-Id", "X-Codex-Turn-Metadata"} {
		if len(headers.Values(key)) > 1 || len(headers.Get(key)) > 4096 {
			return ""
		}
	}
	var client struct {
		Turn string `json:"x-codex-turn-metadata"`
	}
	if raw := input["client_metadata"]; len(raw) > 0 {
		if len(raw) > 8192 || json.Unmarshal(raw, &client) != nil {
			return ""
		}
	}
	parents := []string{headers.Get("X-Codex-Parent-Thread-Id")}
	subagents := []string{headers.Get("X-OpenAI-Subagent")}
	for _, raw := range []string{headers.Get("X-Codex-Turn-Metadata"), client.Turn} {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var turn struct {
			Parent   string `json:"parent_thread_id"`
			Subagent string `json:"subagent_kind"`
		}
		if len(raw) > 4096 || json.Unmarshal([]byte(raw), &turn) != nil {
			return ""
		}
		parents = append(parents, turn.Parent)
		subagents = append(subagents, turn.Subagent)
	}
	parent, subagent := "", ""
	for _, value := range parents {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > 1024 || parent != "" && parent != value {
			return ""
		}
		parent = value
	}
	for _, value := range subagents {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if subagent != "" && subagent != value {
			return ""
		}
		subagent = value
	}
	if subagent != "guardian" && subagent != "review" {
		return ""
	}
	return parent
}
