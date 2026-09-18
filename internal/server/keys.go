package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/apikey"
)

type keyHTTP struct{ service *apikey.Service }

func (h *keyHTTP) available(w http.ResponseWriter) bool {
	if h.service != nil {
		return true
	}
	writeJSON(w, 503, map[string]string{"error": "unavailable"})
	return false
}

func (h *keyHTTP) register(router chi.Router) {
	router.Group(func(keys chi.Router) {
		keys.Use(h.requireAvailable)
		keys.Get("/", h.list)
		keys.Post("/", h.create)
		keys.Post("/{id}/revoke", h.revoke)
	})
}

func (h *keyHTTP) requireAvailable(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.available(w) {
			next.ServeHTTP(w, r)
		}
	})
}

func (h *keyHTTP) list(w http.ResponseWriter, r *http.Request) {
	var cursor int64
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 0 {
			keyError(w, apikey.ErrInput)
			return
		}
		cursor = value
	}
	page, err := h.service.List(r.Context(), sessionUser(r).ID, cursor)
	if err != nil {
		keyError(w, err)
		return
	}
	writeJSON(w, 200, page)
}

func (h *keyHTTP) create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	created, err := h.service.Create(r.Context(), sessionUser(r).ID, input.Name)
	if err != nil {
		keyError(w, err)
		return
	}
	writeJSON(w, 201, created)
}

func (h *keyHTTP) revoke(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		keyError(w, apikey.ErrInput)
		return
	}
	var input struct{}
	if !decodeJSON(w, r, &input) {
		return
	}
	key, err := h.service.Revoke(r.Context(), sessionUser(r).ID, id)
	if err != nil {
		keyError(w, err)
		return
	}
	writeJSON(w, 200, key)
}

func keyError(w http.ResponseWriter, err error) {
	status, code := 503, "unavailable"
	switch {
	case errors.Is(err, apikey.ErrInput):
		status, code = 400, "invalid_input"
	case errors.Is(err, apikey.ErrLimit):
		status, code = 409, "api_key_limit"
	case errors.Is(err, apikey.ErrNotFound):
		status, code = 404, "api_key_not_found"
	case errors.Is(err, apikey.ErrOwnerUnavailable):
		status, code = 401, "unauthorized"
	case errors.Is(err, apikey.ErrInvalidKey):
		status, code = 401, "invalid_api_key"
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	writeJSON(w, status, map[string]string{"error": code})
}
