package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/metrics"
)

// WithMetrics keeps process-wide telemetry outside workspace and browser-session routing.
func WithMetrics(next http.Handler, token string, collector *metrics.Collector, pressure func() (int64, int64)) http.Handler {
	router := chi.NewRouter()
	expected := sha256.Sum256([]byte(token))
	router.Route("/metrics", func(endpoint chi.Router) {
		routeErrors(endpoint)
		endpoint.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				if token == "" || collector == nil {
					writeJSON(w, 404, map[string]string{"error": "not_found"})
					return
				}
				header := r.Header.Get("Authorization")
				provided := sha256.Sum256([]byte(strings.TrimPrefix(header, "Bearer ")))
				if !strings.HasPrefix(header, "Bearer ") || len(header) > 256 || subtle.ConstantTimeCompare(provided[:], expected[:]) != 1 {
					w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
					writeJSON(w, 401, map[string]string{"error": "unauthorized"})
					return
				}
				next.ServeHTTP(w, r)
			})
		})
		endpoint.Get("/", func(w http.ResponseWriter, r *http.Request) {
			var active, queued int64
			if pressure != nil {
				active, queued = pressure()
			}
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			_ = collector.Write(w, active, queued)
		})
	})
	router.Handle("/*", next)
	return router
}
