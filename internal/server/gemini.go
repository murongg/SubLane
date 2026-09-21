package server

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/gateway"
)

func (h *keyHTTP) registerGemini(router chi.Router) {
	router.Use(nativeIdentity, h.requireGeminiKey)
	routeErrorsWith(router, writeGeminiError)
	router.Post("/models/{model}:generateContent", func(w http.ResponseWriter, r *http.Request) { h.proxy(w, r, gateway.Gemini) })
	router.Post("/models/{model}:streamGenerateContent", func(w http.ResponseWriter, r *http.Request) { h.proxy(w, r, gateway.GeminiStream) })
}

func (h *keyHTTP) requireGeminiKey(next http.Handler) http.Handler {
	return h.authenticateKey(next, gateway.Gemini)
}

func geminiError(w http.ResponseWriter, err error) { writeGatewayFailure(w, err, writeGeminiError) }

func geminiErrorBody(status int, code string) map[string]any {
	name := "INTERNAL"
	switch {
	case status == 401:
		name = "UNAUTHENTICATED"
	case status == 403:
		name = "PERMISSION_DENIED"
	case status == 404:
		name = "NOT_FOUND"
	case status == 409:
		name = "ABORTED"
	case status == 429:
		name = "RESOURCE_EXHAUSTED"
	case status == 503 || status == 529:
		name = "UNAVAILABLE"
	case status == 504:
		name = "DEADLINE_EXCEEDED"
	case status >= 400 && status < 500:
		name = "INVALID_ARGUMENT"
	}
	return map[string]any{"error": map[string]any{"code": status, "status": name, "message": strings.ReplaceAll(code, "_", " ")}}
}

func writeGeminiError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, geminiErrorBody(status, code))
}
