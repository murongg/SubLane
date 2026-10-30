package server

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/turnstile"
)

type turnstileHTTP struct{ service *turnstile.Service }

func (h *turnstileHTTP) register(router chi.Router) {
	routeErrors(router)
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if h.service == nil {
				writeJSON(w, 503, map[string]string{"error": "turnstile_unavailable"})
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	router.Get("/", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, h.service.State()) })
	router.Patch("/", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Enabled   *bool   `json:"enabled"`
			SiteKey   *string `json:"site_key"`
			SecretKey *string `json:"secret_key,omitempty"`
		}
		if !decodeJSONLimit(w, r, &input, 2048) {
			return
		}
		if input.Enabled == nil || input.SiteKey == nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_turnstile_settings"})
			return
		}
		state, err := h.service.Save(r.Context(), turnstile.Input{Enabled: *input.Enabled, SiteKey: *input.SiteKey, SecretKey: input.SecretKey})
		if err != nil {
			status, code := 503, "turnstile_unavailable"
			if errors.Is(err, turnstile.ErrInput) {
				status, code = 400, "invalid_turnstile_settings"
			}
			writeJSON(w, status, map[string]string{"error": code})
			return
		}
		writeJSON(w, 200, state)
	})
}

func (h *authHTTP) verifyTurnstile(w http.ResponseWriter, r *http.Request, token string) bool {
	if h.turnstile == nil {
		return true
	}
	origin, err := url.Parse(h.origin(r))
	if err == nil {
		err = h.turnstile.Verify(r.Context(), token, origin.Hostname())
	}
	if err == nil {
		return true
	}
	status, code := 503, "turnstile_unavailable"
	if errors.Is(err, turnstile.ErrRejected) {
		status, code = 403, "turnstile_failed"
	}
	writeJSON(w, status, map[string]string{"error": code})
	return false
}
