package server

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/apikey"
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
	// Malformed filters must not silently broaden a selected-key report to all keys.
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		accountError(w, accounts.ErrInput)
		return
	}
	days := int64(7)
	if value := values.Get("days"); value != "" {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			accountError(w, accounts.ErrInput)
			return
		}
		days = n
	}
	var result gateway.Statistics
	var keyID int64
	if keyValues, supplied := values["key_id"]; supplied {
		if len(keyValues) != 1 || !personal {
			accountError(w, accounts.ErrInput)
			return
		}
		keyID, err = strconv.ParseInt(keyValues[0], 10, 64)
		if err != nil || keyID <= 0 {
			accountError(w, accounts.ErrInput)
			return
		}
	}
	if keyID != 0 {
		result, err = h.gateway.UserKeyStatistics(r.Context(), sessionUser(r).ID, keyID, days)
	} else if personal {
		result, err = h.gateway.UserStatistics(r.Context(), sessionUser(r).ID, days)
	} else {
		result, err = h.gateway.Statistics(r.Context(), days)
	}
	if err != nil {
		if errors.Is(err, apikey.ErrNotFound) {
			keyError(w, err)
			return
		}
		accountError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
