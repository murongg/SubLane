package server

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/upstream"
)

func (h *accountHTTP) checkModel(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Model string `json:"model"`
	}
	if !decodeJSONLimit(w, r, &input, 1024) {
		return
	}
	if h.gateway == nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	result, err := h.gateway.CheckModel(r.Context(), chi.URLParam(r, "id"), input.Model)
	if errors.Is(err, upstream.ErrInput) {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if err != nil {
		accountError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
