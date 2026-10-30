package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/resets"
)

func (h *accountHTTP) resetAvailable(w http.ResponseWriter) bool {
	if h.resets == nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return false
	}
	return true
}

func (h *accountHTTP) resetState(w http.ResponseWriter, r *http.Request) {
	if !h.resetAvailable(w) {
		return
	}
	state, err := h.resets.State(r.Context(), h.tenantID, chi.URLParam(r, "id"))
	if err != nil {
		accountError(w, err)
		return
	}
	writeJSON(w, 200, state)
}

func (h *accountHTTP) resetCards(w http.ResponseWriter, r *http.Request) {
	if h.gateway == nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	cards, err := h.gateway.ResetCards(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		accountError(w, err)
		return
	}
	writeJSON(w, 200, cards)
}
func (h *accountHTTP) resetSettings(w http.ResponseWriter, r *http.Request) {
	if !h.resetAvailable(w) {
		return
	}
	var input resets.Config
	if !decodeJSON(w, r, &input) {
		return
	}
	state, err := h.resets.Save(r.Context(), h.tenantID, chi.URLParam(r, "id"), input)
	if err != nil {
		accountError(w, err)
		return
	}
	writeJSON(w, 200, state)
}
func (h *accountHTTP) consumeReset(w http.ResponseWriter, r *http.Request) { h.runReset(w, r, false) }
func (h *accountHTTP) retryReset(w http.ResponseWriter, r *http.Request)   { h.runReset(w, r, true) }
func (h *accountHTTP) runReset(w http.ResponseWriter, r *http.Request, retry bool) {
	if !h.resetAvailable(w) {
		return
	}
	var input struct{}
	if !decodeJSON(w, r, &input) {
		return
	}
	var state resets.State
	var err error
	if retry {
		state, err = h.resets.Retry(r.Context(), h.tenantID, chi.URLParam(r, "id"))
	} else {
		state, err = h.resets.Run(r.Context(), h.tenantID, chi.URLParam(r, "id"), "manual")
	}
	if err != nil {
		accountError(w, err)
		return
	}
	writeJSON(w, 200, state)
}
