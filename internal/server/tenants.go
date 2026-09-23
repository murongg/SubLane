package server

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/tenants"
)

type tenantHTTP struct{ service *tenants.Service }

func (h *tenantHTTP) register(router chi.Router) {
	router.Get("/", h.list)
	router.Post("/", h.create)
	router.Post("/{id}/members", h.addMember)
}

func (h *tenantHTTP) list(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	values, err := h.service.List(r.Context(), sessionUser(r).ID)
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	writeJSON(w, 200, map[string]any{"tenants": values})
}

func (h *tenantHTTP) create(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	value, err := h.service.Create(r.Context(), sessionUser(r).ID, input.Name)
	if err != nil {
		switch {
		case errors.Is(err, tenants.ErrInput):
			writeJSON(w, 400, map[string]string{"error": "invalid_input"})
		case errors.Is(err, tenants.ErrForbidden):
			writeJSON(w, 403, map[string]string{"error": "forbidden"})
		default:
			writeJSON(w, 503, map[string]string{"error": "unavailable"})
		}
		return
	}
	writeJSON(w, 201, value)
}

func (h *tenantHTTP) addMember(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	tenantID, err := pathID(r)
	if err != nil || tenantID <= 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid_input"})
		return
	}
	var input struct {
		UserID int64        `json:"user_id"`
		Role   tenants.Role `json:"role"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := h.service.AddMember(r.Context(), sessionUser(r).ID, tenantID, input.UserID, input.Role); err != nil {
		switch {
		case errors.Is(err, tenants.ErrInput):
			writeJSON(w, 400, map[string]string{"error": "invalid_input"})
		case errors.Is(err, tenants.ErrForbidden):
			writeJSON(w, 403, map[string]string{"error": "forbidden"})
		default:
			writeJSON(w, 503, map[string]string{"error": "unavailable"})
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
