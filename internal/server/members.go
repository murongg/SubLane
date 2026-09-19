package server

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (h *authHTTP) registerMembers(router chi.Router) {
	routeErrors(router)
	router.Get("/", h.listMembers)
	router.Post("/", h.createMember)
	router.Patch("/{id}", h.memberStatus)
	router.With(h.throttleLogin).Post("/{id}/password", h.resetMemberPassword)
}

func (h *authHTTP) listMembers(w http.ResponseWriter, r *http.Request) {
	var cursor int64
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 0 {
			writeJSON(w, 400, map[string]string{"error": "invalid_input"})
			return
		}
		cursor = value
	}
	page, err := h.service.ListMembers(r.Context(), cursor)
	if err != nil {
		authError(w, err)
		return
	}
	writeJSON(w, 200, page)
}

func (h *authHTTP) createMember(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}
	member, err := h.service.CreateMember(r.Context(), input.Username, input.Password)
	if err != nil {
		authError(w, err)
		return
	}
	writeJSON(w, 201, member)
}

func (h *authHTTP) memberStatus(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid_input"})
		return
	}
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Enabled == nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_input"})
		return
	}
	member, err := h.service.SetMemberEnabled(r.Context(), id, *input.Enabled)
	if err != nil {
		authError(w, err)
		return
	}
	writeJSON(w, 200, member)
}
