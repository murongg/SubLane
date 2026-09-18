package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestRoutesAndAssetBoundary(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("<html>synthetic app</html>")}, "assets/app.js": {Data: []byte("/* synthetic bundle */")}}
	h := New(Options{Assets: assets, Version: "test", StartedAt: time.Now(), Ping: func(context.Context) error { return nil }})
	for _, tc := range []struct {
		method, path string
		status       int
		content      string
	}{
		{"GET", "/healthz", 200, `"status":"ok"`},
		{"GET", "/readyz", 200, `"status":"ready"`},
		{"GET", "/api/system", 503, `"error":"unavailable"`},
		{"GET", "/accounts", 200, "synthetic app"},
		{"GET", "/assets/app.js", 200, "synthetic bundle"},
		{"GET", "/assets/missing.js", 404, "not found"},
		{"GET", "/api/missing", 503, `"error":"unavailable"`},
		{"GET", "/api", 404, `"error"`},
		{"GET", "/v1/responses", 404, `"error"`},
		{"POST", "/accounts", 405, "Method Not Allowed"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.content) {
				t.Fatalf("got %d %s", w.Code, w.Body.String())
			}
			if w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing content type protection")
			}
		})
	}
}

func TestDatabaseFailureIsNotHealthyOrExposed(t *testing.T) {
	h := New(Options{Ping: func(context.Context) error { return errors.New("synthetic private path") }, StartedAt: time.Now()})
	for _, path := range []string{"/readyz", "/api/system"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), "private") {
			t.Fatalf("got %d %s", w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Fatal("liveness must be independent of database readiness")
	}
}
