package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/apikey"
)

func (h *accountHTTP) catalog(w http.ResponseWriter, r *http.Request) { h.readCatalog(w, r, false) }
func (h *accountHTTP) refreshCatalog(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decodeJSON(w, r, &input) {
		return
	}
	h.readCatalog(w, r, true)
}
func (h *accountHTTP) readCatalog(w http.ResponseWriter, r *http.Request, force bool) {
	if h.gateway == nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	value, err := h.gateway.AccountCatalog(r.Context(), chi.URLParam(r, "id"), force)
	if err != nil {
		accountError(w, err)
		return
	}
	writeJSON(w, 200, value)
}

func (h *groupHTTP) catalog(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid_group_input"})
		return
	}
	if h.gateway == nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	value, err := h.gateway.GroupCatalog(r.Context(), sessionUser(r).ID, id, false)
	if err != nil {
		groupError(w, err)
		return
	}
	writeJSON(w, 200, value)
}

func (h *keyHTTP) catalog(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		keyError(w, apikey.ErrInput)
		return
	}
	userID := sessionUser(r).ID
	groupID, err := h.service.CatalogGroup(r.Context(), userID, id)
	if err != nil {
		keyError(w, err)
		return
	}
	if h.gateway == nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	value, err := h.gateway.GroupCatalog(r.Context(), userID, groupID, false)
	if err != nil {
		groupError(w, err)
		return
	}
	if _, err := h.service.CatalogGroup(r.Context(), userID, id); err != nil {
		keyError(w, err)
		return
	}
	writeJSON(w, 200, value)
}
