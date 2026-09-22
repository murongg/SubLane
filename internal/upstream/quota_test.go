package upstream

import (
	"net/http"
	"testing"
	"time"
)

func TestResponseQuotaValidatesWindows(t *testing.T) {
	now := time.Unix(1900000000, 0)
	raw := []byte(`{"type":"codex.rate_limits","rate_limits":{"primary":{"used_percent":12.5,"window_minutes":300,"reset_after_seconds":1800},"secondary":{"used_percent":30,"window_minutes":10080,"reset_at":1900086400}}}`)
	u, ok := QuotaEvent(raw, now)
	if !ok || len(u.Limits) != 1 || len(u.Limits[0].Windows) != 2 || *u.Limits[0].Windows[0].ResetAt != now.Unix()+1800 || *u.Limits[0].Windows[0].UsedPercent != 12.5 {
		t.Fatalf("invalid response observation: %+v", u)
	}
	for _, raw := range []string{
		`{"type":"response.completed","rate_limits":{"primary":{"used_percent":50,"reset_at":1900086400}}}`,
		`{"type":"codex.rate_limits","rate_limits":{"primary":{"reset_at":1900086400}}}`,
		`{"type":"codex.rate_limits","rate_limits":{"primary":{"used_percent":5}}}`,
		`{"type":"codex.rate_limits","rate_limits":{"primary":{"used_percent":-1,"reset_at":1900086400}}}`,
		`{"type":"codex.rate_limits","rate_limits":{"primary":{"used_percent":5,"reset_at":1800000000}}}`,
	} {
		if _, ok := QuotaEvent([]byte(raw), now); ok {
			t.Fatalf("accepted invalid quota: %s", raw)
		}
	}
	h := http.Header{}
	h.Set("X-Codex-Primary-Used-Percent", "12.5")
	h.Set("X-Codex-Primary-Reset-At", "1900001800")
	h.Set("X-Codex-Primary-Window-Minutes", "300")
	if u, ok := QuotaHeaders(h, now); !ok || *u.Limits[0].Windows[0].UsedPercent != 12.5 {
		t.Fatal("headers not observed")
	}
	h.Set("X-Codex-Primary-Used-Percent", "NaN")
	if _, ok := QuotaHeaders(h, now); ok {
		t.Fatal("NaN quota admitted")
	}
}
