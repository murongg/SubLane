package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/murongg/SubLane/internal/auth"
)

type Options struct {
	Assets    fs.FS
	Version   string
	StartedAt time.Time
	Ping      func(context.Context) error
	Auth      *auth.Service
	PublicURL string
}

func New(o Options) http.Handler {
	mux := http.NewServeMux()
	management := http.NewServeMux()
	login := &authHTTP{service: o.Auth, publicURL: o.PublicURL, limiter: newLoginLimiter()}
	login.register(mux)
	mux.Handle("/api/", login.require(management))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !ready(r.Context(), o.Ping) {
			writeJSON(w, 503, map[string]string{"error": "storage_unavailable"})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ready"})
	})
	management.HandleFunc("GET /api/system", func(w http.ResponseWriter, r *http.Request) {
		if !ready(r.Context(), o.Ping) {
			writeJSON(w, 503, map[string]string{"error": "storage_unavailable"})
			return
		}
		writeJSON(w, 200, map[string]any{
			"name": "SubLane", "version": o.Version, "status": "ok", "uptime_seconds": max(0, int64(time.Since(o.StartedAt).Seconds())),
			"storage": map[string]string{"engine": "sqlite", "status": "ready"},
			"gateway": map[string]string{"provider": "codex", "status": "not_configured"},
		})
	})
	// Unknown API endpoints must never fall through to the SPA with a misleading 200.
	management.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
	})
	for _, prefix := range []string{"/api", "/v1", "/v1/", "/v0", "/v0/"} {
		mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, 404, map[string]string{"error": "not_found"})
		})
	}
	mux.Handle("/", assets(o.Assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		mux.ServeHTTP(w, r)
	})
}

func ready(parent context.Context, ping func(context.Context) error) bool {
	if ping == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	return ping(ctx) == nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func assets(files fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if files == nil {
			writeJSON(w, 503, map[string]string{"error": "frontend_not_built", "message": "Use the Vite development server or make build."})
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		info, err := fs.Stat(files, name)
		if err != nil || info.IsDir() {
			if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
			name = "index.html"
		}
		w.Header().Set("Cache-Control", "no-cache")
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFileFS(w, r, files, name)
	})
}
