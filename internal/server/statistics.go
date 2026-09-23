package server

import (
	"net/http"
	"strconv"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/gateway"
)

type memberHTTP struct{ gateway *gateway.Service }

func (h *memberHTTP) available(w http.ResponseWriter) bool {
	if h.gateway != nil {
		return true
	}
	writeJSON(w, 503, map[string]string{"error": "unavailable"})
	return false
}
func (h *memberHTTP) ownLimits(w http.ResponseWriter, r *http.Request) {
	h.readLimits(w, r, sessionUser(r).ID)
}
func (h *memberHTTP) limits(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		accountError(w, accounts.ErrInput)
		return
	}
	h.readLimits(w, r, id)
}
func (h *memberHTTP) readLimits(w http.ResponseWriter, r *http.Request, id int64) {
	if !h.available(w) {
		return
	}
	value, err := h.gateway.MemberLimits(r.Context(), id)
	if err != nil {
		authError(w, err)
		return
	}
	writeJSON(w, 200, value)
}
func (h *memberHTTP) updateLimits(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	id, err := pathID(r)
	if err != nil || id <= 0 {
		accountError(w, accounts.ErrInput)
		return
	}
	var input struct {
		RPM         *int64 `json:"requests_per_minute"`
		Concurrency *int64 `json:"max_concurrency"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.RPM == nil || input.Concurrency == nil {
		accountError(w, accounts.ErrInput)
		return
	}
	if err := h.gateway.SetMemberLimits(r.Context(), id, *input.RPM, *input.Concurrency); err != nil {
		if err == accounts.ErrInput {
			accountError(w, err)
		} else {
			authError(w, err)
		}
		return
	}
	h.readLimits(w, r, id)
}
func (h *memberHTTP) usage(w http.ResponseWriter, r *http.Request)    { h.statistics(w, r, false) }
func (h *memberHTTP) ownUsage(w http.ResponseWriter, r *http.Request) { h.statistics(w, r, true) }
func (h *memberHTTP) statistics(w http.ResponseWriter, r *http.Request, personal bool) {
	if !h.available(w) {
		return
	}
	days := int64(7)
	if value := r.URL.Query().Get("days"); value != "" {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			accountError(w, accounts.ErrInput)
			return
		}
		days = n
	}
	var result gateway.Statistics
	var err error
	if personal {
		result, err = h.gateway.UserStatistics(r.Context(), sessionUser(r).ID, days)
	} else {
		result, err = h.gateway.Statistics(r.Context(), days)
	}
	if err != nil {
		accountError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
