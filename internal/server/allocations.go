package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/gateway"
)

type allocationHTTP struct{ gateway *gateway.Service }

func (h *allocationHTTP) prices(w http.ResponseWriter, r *http.Request) {
	if h.gateway == nil || h.gateway.Pricing() == nil {
		writeJSON(w, 200, map[string]any{"prices": map[string]any{}})
		return
	}
	models := r.URL.Query()["model"]
	if len(models) == 0 {
		allocationError(w, allocations.ErrInput)
		return
	}
	result := map[string]any{}
	for _, model := range models {
		if strings.TrimSpace(model) == "" {
			continue
		}
		if price, ok := h.gateway.Pricing().Lookup(model); ok {
			result[model] = price
		}
	}
	writeJSON(w, 200, map[string]any{"prices": result})
}

func (h *allocationHTTP) available(w http.ResponseWriter, r *http.Request) bool {
	if h.gateway != nil {
		if err := h.gateway.PrepareAllocations(r.Context()); err != nil {
			writeJSON(w, 503, map[string]string{"error": "token_accounting_unavailable"})
			return false
		}
		return true
	}
	writeJSON(w, 503, map[string]string{"error": "unavailable"})
	return false
}
func allocationError(w http.ResponseWriter, err error) {
	status, code := 503, "unavailable"
	switch {
	case errors.Is(err, allocations.ErrInput):
		status, code = 400, err.Error()
	case errors.Is(err, allocations.ErrNotFound):
		status, code = 404, err.Error()
	case errors.Is(err, allocations.ErrUnavailable):
		status, code = 403, err.Error()
	case errors.Is(err, allocations.ErrPoolConflict), errors.Is(err, allocations.ErrSettlement), errors.Is(err, allocations.ErrPending), errors.Is(err, allocations.ErrUnpriced):
		status, code = 409, err.Error()
	case errors.Is(err, allocations.ErrSnapshot):
		status, code = 409, err.Error()
	}
	writeJSON(w, status, map[string]string{"error": code})
}
func (h *allocationHTTP) registerTeams(r chi.Router) {
	r.Get("/", h.teams)
	r.Post("/", func(w http.ResponseWriter, r *http.Request) { h.saveTeam(w, r, 0) })
	r.Patch("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil || id <= 0 {
			allocationError(w, allocations.ErrInput)
			return
		}
		h.saveTeam(w, r, id)
	})
}
func (h *allocationHTTP) teams(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	rows, err := h.gateway.Allocations().Teams(r.Context())
	if err != nil {
		allocationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"teams": rows})
}
func (h *allocationHTTP) saveTeam(w http.ResponseWriter, r *http.Request, id int64) {
	if !h.available(w, r) {
		return
	}
	var in allocations.TeamInput
	if !decodeJSON(w, r, &in) {
		return
	}
	value, err := h.gateway.Allocations().SaveTeam(r.Context(), id, in)
	if err != nil {
		allocationError(w, err)
		return
	}
	status := 200
	if id == 0 {
		status = 201
	}
	writeJSON(w, status, value)
}
func (h *allocationHTTP) register(r chi.Router) {
	r.Get("/prices", h.prices)
	r.Get("/", h.list)
	r.Post("/", func(w http.ResponseWriter, r *http.Request) { h.save(w, r, 0) })
	r.Get("/{id}", h.detail)
	r.Patch("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil || id <= 0 {
			allocationError(w, allocations.ErrInput)
			return
		}
		h.save(w, r, id)
	})
	r.Post("/{id}/refresh", h.refresh)
	r.Post("/{id}/settle", h.settle)
	r.Post("/{id}/reserve", h.reserve)
	r.Post("/{id}/enabled", h.enabled)
}
func (h *allocationHTTP) list(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	rows, err := h.gateway.Allocations().Schemes(r.Context())
	if err != nil {
		allocationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"schemes": rows})
}
func (h *allocationHTTP) save(w http.ResponseWriter, r *http.Request, id int64) {
	if !h.available(w, r) {
		return
	}
	var in allocations.SchemeInput
	if !decodeJSONLimit(w, r, &in, 64<<10) {
		return
	}
	value, err := h.gateway.Allocations().SaveScheme(r.Context(), id, in)
	if err != nil {
		allocationError(w, err)
		return
	}
	status := 200
	if id == 0 {
		status = 201
	}
	writeJSON(w, status, value)
}
func (h *allocationHTTP) detail(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	id, err := pathID(r)
	if err != nil || id <= 0 {
		allocationError(w, allocations.ErrInput)
		return
	}
	value, err := h.gateway.Allocations().Detail(r.Context(), id, 0)
	if err != nil {
		allocationError(w, err)
		return
	}
	writeJSON(w, 200, value)
}
func (h *allocationHTTP) own(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	values, err := h.gateway.Allocations().Own(r.Context(), sessionUser(r).ID)
	if err != nil {
		allocationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"schemes": values})
}
func (h *allocationHTTP) refresh(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	id, err := pathID(r)
	if err != nil || id <= 0 {
		allocationError(w, allocations.ErrInput)
		return
	}
	var in struct{}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err = h.gateway.RefreshAllocation(r.Context(), id); err != nil {
		allocationError(w, err)
		return
	}
	h.detail(w, r)
}
func (h *allocationHTTP) settle(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	id, err := pathID(r)
	if err != nil || id <= 0 {
		allocationError(w, allocations.ErrInput)
		return
	}
	var in struct {
		RequestID string          `json:"request_id"`
		Input     *int64          `json:"input"`
		Output    *int64          `json:"output"`
		Cached    *int64          `json:"cached"`
		Points    map[int64]int64 `json:"points"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Input == nil || in.Output == nil || in.Cached == nil {
		allocationError(w, allocations.ErrInput)
		return
	}
	err = h.gateway.Allocations().Settle(r.Context(), id, in.RequestID, allocations.Completion{Input: *in.Input, Output: *in.Output, Cached: *in.Cached}, in.Points)
	if err != nil {
		allocationError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (h *allocationHTTP) reserve(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	id, err := pathID(r)
	if err != nil || id <= 0 {
		allocationError(w, allocations.ErrInput)
		return
	}
	var in struct {
		Points int64 `json:"points"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err = h.gateway.Allocations().ReserveUnassigned(r.Context(), id, in.Points); err != nil {
		allocationError(w, err)
		return
	}
	w.WriteHeader(204)
}

func (h *allocationHTTP) enabled(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	id, err := pathID(r)
	if err != nil || id <= 0 {
		allocationError(w, allocations.ErrInput)
		return
	}
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Enabled == nil {
		allocationError(w, allocations.ErrInput)
		return
	}
	if err = h.gateway.Allocations().SetEnabled(r.Context(), id, *in.Enabled); err != nil {
		allocationError(w, err)
		return
	}
	w.WriteHeader(204)
}
