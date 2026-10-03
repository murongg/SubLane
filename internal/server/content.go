package server

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/content"
)

type contentHTTP struct{ service *content.Service }

func (h *contentHTTP) register(router chi.Router) {
	routeErrors(router)
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if h.service == nil {
				writeJSON(w, 503, map[string]string{"error": "content_check_unavailable"})
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		state, err := h.service.State(r.Context())
		h.respond(w, state, err)
	})
	router.Put("/", func(w http.ResponseWriter, r *http.Request) {
		var input content.Input
		if !decodeJSONLimit(w, r, &input, 131072) {
			return
		}
		state, err := h.service.Update(r.Context(), input)
		h.respond(w, state, err)
	})
	router.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Rule   content.RuleInput `json:"rule"`
			Sample string            `json:"sample"`
		}
		if !decodeJSONLimit(w, r, &input, 524288) {
			return
		}
		matched, err := h.service.Test(r.Context(), input.Rule, input.Sample)
		h.respond(w, map[string]bool{"matched": matched}, err)
	})
}
func (h *contentHTTP) respond(w http.ResponseWriter, value any, err error) {
	if err == nil {
		writeJSON(w, 200, value)
		return
	}
	status := 503
	code := "content_check_unavailable"
	if errors.Is(err, content.ErrInput) {
		status = 400
		code = "invalid_content_rules"
	}
	if errors.Is(err, content.ErrConflict) {
		status = 409
		code = "content_rules_changed"
	}
	writeJSON(w, status, map[string]string{"error": code})
}
