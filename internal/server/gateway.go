package server

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/apikey"
)

func (h *keyHTTP) registerGateway(router chi.Router) {
	routeErrors(router)
	// Authenticate the entire gateway router, including unknown paths and unsupported methods.
	router.Use(h.requireKey)
	router.Get("/models", gatewayUnavailable)
	router.Post("/responses", gatewayUnavailable)
	router.Post("/chat/completions", gatewayUnavailable)
}

func (h *keyHTTP) requireKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme, secret, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			keyError(w, apikey.ErrInvalidKey)
			return
		}
		if !h.available(w) {
			return
		}
		if _, err := h.service.Authenticate(r.Context(), strings.TrimSpace(secret)); err != nil {
			keyError(w, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func gatewayUnavailable(w http.ResponseWriter, r *http.Request) {
	// Authentication is available before the provider adapter; never imply a model request was forwarded.
	writeJSON(w, 501, map[string]string{"error": "gateway_not_configured"})
}
