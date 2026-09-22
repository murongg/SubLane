package upstream

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"
)

type responseQuotaWindow struct {
	Used    *float64 `json:"used_percent"`
	Minutes *float64 `json:"window_minutes"`
	Reset   *int64   `json:"reset_at"`
	After   *int64   `json:"reset_after_seconds"`
}
type responseQuota struct {
	Allowed   *bool                `json:"allowed"`
	Reached   *bool                `json:"limit_reached"`
	Primary   *responseQuotaWindow `json:"primary"`
	Secondary *responseQuotaWindow `json:"secondary"`
}

// QuotaEvent reads metadata only; malformed observations must not break model output.
func QuotaEvent(raw []byte, now time.Time) (Usage, bool) {
	var event struct {
		Type   string         `json:"type"`
		Limits *responseQuota `json:"rate_limits"`
	}
	if len(raw) > 16<<10 || json.Unmarshal(raw, &event) != nil || event.Type != "codex.rate_limits" || event.Limits == nil {
		return Usage{}, false
	}
	return event.Limits.usage(now)
}

func QuotaHeaders(h http.Header, now time.Time) (Usage, bool) {
	q := responseQuota{}
	for i, prefix := range []string{"X-Codex-Primary", "X-Codex-Secondary"} {
		if h.Get(prefix+"-Used-Percent") == "" {
			continue
		}
		w := &responseQuotaWindow{}
		value, err := strconv.ParseFloat(h.Get(prefix+"-Used-Percent"), 64)
		if err != nil {
			return Usage{}, false
		}
		w.Used = &value
		if text := h.Get(prefix + "-Window-Minutes"); text != "" {
			v, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return Usage{}, false
			}
			w.Minutes = &v
		}
		for suffix, target := range map[string]**int64{"-Reset-At": &w.Reset, "-Reset-After-Seconds": &w.After} {
			if text := h.Get(prefix + suffix); text != "" {
				v, err := strconv.ParseInt(text, 10, 64)
				if err != nil {
					return Usage{}, false
				}
				*target = &v
			}
		}
		if i == 0 {
			q.Primary = w
		} else {
			q.Secondary = w
		}
	}
	for key, target := range map[string]**bool{"X-Codex-Allowed": &q.Allowed, "X-Codex-Limit-Reached": &q.Reached} {
		if text := h.Get(key); text != "" {
			v, err := strconv.ParseBool(text)
			if err != nil {
				return Usage{}, false
			}
			*target = &v
		}
	}
	return q.usage(now)
}

func (q responseQuota) usage(now time.Time) (Usage, bool) {
	limit := UsageLimit{Name: "", Allowed: q.Allowed, LimitReached: q.Reached, Windows: []UsageWindow{}}
	for i, w := range []*responseQuotaWindow{q.Primary, q.Secondary} {
		if w == nil {
			continue
		}
		if w.Used == nil || *w.Used < 0 || math.IsNaN(*w.Used) || math.IsInf(*w.Used, 0) {
			return Usage{}, false
		}
		reset := w.Reset
		if reset == nil && w.After != nil && *w.After >= 0 && *w.After <= 366*86400 {
			v := now.Unix() + *w.After
			reset = &v
		}
		if reset == nil || *reset <= now.Unix() || *reset > 253402300799 {
			return Usage{}, false
		}
		window := UsageWindow{Kind: "primary", UsedPercent: w.Used, ResetAt: reset}
		if i == 1 {
			window.Kind = "secondary"
		}
		if w.Minutes != nil {
			if *w.Minutes <= 0 || *w.Minutes > 366*1440 || math.IsNaN(*w.Minutes) || math.IsInf(*w.Minutes, 0) {
				return Usage{}, false
			}
			seconds := int64(*w.Minutes * 60)
			window.WindowSeconds = &seconds
		}
		limit.Windows = append(limit.Windows, window)
	}
	return Usage{UpdatedAt: now.Unix(), Limits: []UsageLimit{limit}}, len(limit.Windows) > 0
}
