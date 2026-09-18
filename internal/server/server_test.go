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

	"github.com/go-chi/chi/v5"
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
		{"GET", "/v1/responses", 401, `"error"`},
		{"POST", "/accounts", 405, `"error":"method_not_allowed"`},
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

func TestAPIMethodAndFallbackBoundaries(t *testing.T) {
	h := authFixture(t, "")
	setup := request(h, "POST", "/api/auth/setup", "http://example.test", map[string]string{"username": "owner-test", "password": "owner pass 42"}, nil)
	if setup.Code != 201 {
		t.Fatalf("setup: %d", setup.Code)
	}
	owner := setup.Result().Cookies()[0]
	for _, tc := range []struct {
		method, path, allow string
		status              int
	}{
		{"POST", "/api/system", "GET, HEAD", 405},
		{"POST", "/healthz", "GET, HEAD", 405},
		{"POST", "/readyz", "GET, HEAD", 405},
		{"PUT", "/api/members", "GET, POST", 405},
		{"GET", "/api/missing", "", 404},
		{"GET", "/api/auth/missing", "", 404},
		{"GET", "/api/keys/42/missing", "", 404},
		{"GET", "/api/", "", 404},
		{"GET", "/v0/missing", "", 404},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := request(h, tc.method, tc.path, "http://example.test", nil, owner)
			if w.Code != tc.status || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || w.Header().Get("Allow") != tc.allow {
				t.Fatalf("status=%d content-type=%s allow=%s", w.Code, w.Header().Get("Content-Type"), w.Header().Get("Allow"))
			}
		})
	}
	for _, path := range []string{"/api/missing", "/api/auth/missing", "/api/keys/42/missing", "/api/"} {
		if w := request(h, "GET", path, "", nil, nil); w.Code != 401 {
			t.Fatalf("anonymous API fallback %s: %d", path, w.Code)
		}
	}
	for _, path := range []string{"/healthz", "/readyz", "/api/system"} {
		if w := request(h, "HEAD", path, "", nil, owner); w.Code != 200 {
			t.Fatalf("HEAD %s: %d", path, w.Code)
		}
	}
}

func TestChiMethodErrorsUseRegisteredRoutes(t *testing.T) {
	router := chi.NewRouter()
	routeErrors(router)
	calls := 0
	router.Route("/items", func(items chi.Router) {
		routeErrors(items)
		items.Group(func(protected chi.Router) {
			protected.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					next.ServeHTTP(w, r)
				})
			})
			protected.Get("/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
			protected.Patch("/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
		})
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/items/42", nil))
	if w.Code != 405 || w.Header().Get("Allow") != "GET, PATCH" || !strings.Contains(w.Body.String(), `"error":"method_not_allowed"`) {
		t.Fatalf("method response: %d %s %s", w.Code, w.Header().Get("Allow"), w.Body.String())
	}
	if calls != 0 {
		t.Fatal("computing allowed methods must not run handlers or their middleware")
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/items/42/missing", nil))
	if w.Code != 404 || !strings.Contains(w.Body.String(), `"error":"not_found"`) {
		t.Fatalf("fallback: %d %s", w.Code, w.Body.String())
	}
}
