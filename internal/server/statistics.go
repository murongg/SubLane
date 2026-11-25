package server

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

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
	period := gateway.StatisticsPeriod{Days: 7}
	start, hasStart := values["start_date"]
	end, hasEnd := values["end_date"]
	dayValues, hasDays := values["days"]
	switch {
	case hasStart || hasEnd:
		if !personal || hasDays || len(start) != 1 || len(end) != 1 {
			accountError(w, accounts.ErrInput)
			return
		}
		from, fromErr := time.Parse(time.DateOnly, start[0])
		to, toErr := time.Parse(time.DateOnly, end[0])
		if fromErr != nil || toErr != nil || from.Format(time.DateOnly) != start[0] || to.Format(time.DateOnly) != end[0] {
			accountError(w, accounts.ErrInput)
			return
		}
		period = gateway.StatisticsPeriod{StartDate: from, EndDate: to}
	case hasDays:
		if len(dayValues) != 1 {
			accountError(w, accounts.ErrInput)
			return
		}
		if dayValues[0] != "" {
			period.Days, err = strconv.ParseInt(dayValues[0], 10, 64)
			if err != nil {
				accountError(w, accounts.ErrInput)
				return
			}
		}
	}
	include, includeKeys := values["include_keys"]
	if includeKeys && (!personal || len(include) != 1 || include[0] != "1") {
		accountError(w, accounts.ErrInput)
		return
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
	if personal {
		result, err = h.gateway.PersonalStatistics(r.Context(), sessionUser(r).ID, keyID, period, includeKeys)
	} else {
		result, err = h.gateway.Statistics(r.Context(), period.Days)
	}
	if err != nil {
		if errors.Is(err, gateway.ErrUsageExportLimit) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		if errors.Is(err, apikey.ErrNotFound) {
			keyError(w, err)
			return
		}
		accountError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
