package server

import (
	"net/http"
	"strings"

	"github.com/murongg/SubLane/internal/gateway"
)

func (h *keyHTTP) requireMessagesKey(next http.Handler) http.Handler {
	return h.authenticateKey(next, gateway.Messages)
}

func messagesError(w http.ResponseWriter, err error) { writeGatewayFailure(w, err, writeMessagesError) }

func messagesErrorBody(w http.ResponseWriter, status int, code string) map[string]any {
	kind := "api_error"
	switch {
	case status == 401:
		kind = "authentication_error"
	case status == 403:
		kind = "permission_error"
	case status == 404:
		kind = "not_found_error"
	case status == 409:
		kind = "conflict_error"
	case status == 413:
		kind = "request_too_large"
	case status == 429:
		kind = "rate_limit_error"
	case status == 503 || status == 529:
		kind = "overloaded_error"
	case status == 504:
		kind = "timeout_error"
	case status >= 400 && status < 500:
		kind = "invalid_request_error"
	}
	return map[string]any{"type": "error", "error": map[string]string{"type": kind, "message": strings.ReplaceAll(code, "_", " ")}, "request_id": w.Header().Get("Request-Id")}
}

func writeMessagesError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, messagesErrorBody(w, status, code))
}
