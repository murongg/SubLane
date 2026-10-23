package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/metrics"
)

func TestMetricsRequireDedicatedBearerBeforeMethodsAndPaths(t *testing.T) {
	key := strings.Repeat("synthetic", 5)
	called := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	h := WithMetrics(next, key, &metrics.Collector{}, func() (int64, int64) { called++; return 2, 3 })
	for _, test := range []struct {
		method, path, token string
		status              int
	}{
		{"GET", "/metrics", "", 401}, {"POST", "/metrics", "", 401}, {"GET", "/metrics/missing", "", 401},
		{"GET", "/metrics", "wrong", 401}, {"GET", "/metrics", key, 200}, {"POST", "/metrics", key, 405}, {"GET", "/metrics/missing", key, 404}, {"GET", "/api/auth/state", "", 204},
	} {
		r := httptest.NewRequest(test.method, test.path, nil)
		if test.token != "" {
			r.Header.Set("Authorization", "Bearer "+test.token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatal(test.path, w.Code, test.status, w.Body.String())
		}
		if w.Code == 200 && (!strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") || !strings.Contains(w.Body.String(), "sublane_gateway_in_flight 2")) {
			t.Fatal("invalid exposition", w.Body.String())
		}
	}
	if called != 1 {
		t.Fatal("unauthorized request reached gauges", called)
	}
	disabled := WithMetrics(next, "", nil, nil)
	w := httptest.NewRecorder()
	disabled.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != 404 {
		t.Fatal("disabled metrics exposed", w.Code)
	}
}
