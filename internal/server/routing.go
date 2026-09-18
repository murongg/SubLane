package server

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

func notFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 404, map[string]string{"error": "not_found"})
}

func routeErrors(router chi.Router) {
	router.NotFound(notFound)
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		path := chi.RouteContext(r.Context()).RoutePath
		if path == "" {
			path = r.URL.Path
			if r.URL.RawPath != "" {
				path = r.URL.RawPath
			}
		}
		// A custom chi 405 handler must supply Allow itself. Match uses the registered routes without executing middleware.
		var allowed []string
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodConnect, http.MethodTrace} {
			if router.Match(chi.NewRouteContext(), method, path) {
				allowed = append(allowed, method)
			}
		}
		if len(allowed) > 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
		}
		writeJSON(w, 405, map[string]string{"error": "method_not_allowed"})
	})
}

func pathID(r *http.Request) (int64, error) {
	value := chi.URLParam(r, "id")
	// chi matches RawPath when present; decode once to preserve the previous ServeMux parameter behavior.
	if r.URL.RawPath != "" {
		decoded, err := url.PathUnescape(value)
		if err != nil {
			return 0, err
		}
		value = decoded
	}
	return strconv.ParseInt(value, 10, 64)
}
