package server

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/murongg/SubLane/internal/auth"
)

const sessionCookie = "sublane_session"

type authHTTP struct {
	service   *auth.Service
	publicURL string
	limiter   *loginLimiter
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *authHTTP) register(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/state", h.state)
	mux.HandleFunc("/api/auth/setup", func(w http.ResponseWriter, r *http.Request) { h.authenticate(w, r, true) })
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) { h.authenticate(w, r, false) })
	mux.HandleFunc("/api/auth/logout", h.logout)
}

func method(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Method == want {
		return true
	}
	w.Header().Set("Allow", want)
	writeJSON(w, 405, map[string]string{"error": "method_not_allowed"})
	return false
}

func (h *authHTTP) available(w http.ResponseWriter) bool {
	if h.service != nil {
		return true
	}
	writeJSON(w, 503, map[string]string{"error": "unavailable"})
	return false
}

func token(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (h *authHTTP) secure(r *http.Request) bool {
	return r.TLS != nil || strings.HasPrefix(h.publicURL, "https://")
}

func (h *authHTTP) sameOrigin(w http.ResponseWriter, r *http.Request) bool {
	expected := h.publicURL
	if expected == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		expected = scheme + "://" + r.Host
	}
	if r.Header.Get("Origin") == expected && r.Header.Get("Sec-Fetch-Site") != "cross-site" {
		return true
	}
	writeJSON(w, 403, map[string]string{"error": "origin_rejected"})
	return false
}

func decodeJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	// Authentication bodies are small; a slow sender must not hold a handler indefinitely.
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer controller.SetReadDeadline(time.Time{})
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/json" {
		writeJSON(w, 415, map[string]string{"error": "json_required"})
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(into)
	if err == nil {
		var extra any
		err = decoder.Decode(&extra)
		if errors.Is(err, io.EOF) {
			return true
		}
	}
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		writeJSON(w, 413, map[string]string{"error": "body_too_large"})
	} else {
		writeJSON(w, 400, map[string]string{"error": "invalid_input"})
	}
	return false
}

func (h *authHTTP) state(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") || !h.available(w) {
		return
	}
	state, err := h.service.State(r.Context(), token(r))
	if err != nil {
		authError(w, err)
		return
	}
	writeJSON(w, 200, state)
}

func (h *authHTTP) authenticate(w http.ResponseWriter, r *http.Request, setup bool) {
	if !method(w, r, "POST") || !h.available(w) || !h.sameOrigin(w, r) {
		return
	}
	if retry := h.limiter.allow(peerAddress(r.RemoteAddr)); retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		writeJSON(w, 429, map[string]string{"error": "rate_limited"})
		return
	}
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}
	var session auth.Session
	var err error
	status := 200
	if setup {
		session, err = h.service.Setup(r.Context(), input.Username, input.Password)
		status = 201
	} else {
		session, err = h.service.Login(r.Context(), input.Username, input.Password)
	}
	if err != nil {
		authError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: session.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: h.secure(r), MaxAge: int(auth.SessionTTL.Seconds()), Expires: session.ExpiresAt})
	writeJSON(w, status, auth.State{Initialized: true, User: &session.User})
}

func (h *authHTTP) logout(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") || !h.available(w) || !h.sameOrigin(w, r) {
		return
	}
	var input struct{}
	if !decodeJSON(w, r, &input) {
		return
	}
	// Never tell the browser logout succeeded before the durable revocation has completed.
	if err := h.service.Revoke(r.Context(), token(r)); err != nil {
		authError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: h.secure(r), MaxAge: -1, Expires: time.Unix(1, 0)})
	writeJSON(w, 200, auth.State{Initialized: true})
}

func (h *authHTTP) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.available(w) {
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && !h.sameOrigin(w, r) {
			return
		}
		state, err := h.service.State(r.Context(), token(r))
		if err != nil {
			authError(w, err)
			return
		}
		if state.User == nil {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func authError(w http.ResponseWriter, err error) {
	status, code := 503, "unavailable"
	switch {
	case errors.Is(err, auth.ErrInitialized):
		status, code = 409, "already_initialized"
	case errors.Is(err, auth.ErrCredentials):
		status, code = 401, "invalid_credentials"
	case errors.Is(err, auth.ErrInput):
		status, code = 400, "invalid_input"
	case errors.Is(err, auth.ErrBusy):
		status, code = 429, "auth_busy"
		w.Header().Set("Retry-After", "1")
	}
	writeJSON(w, status, map[string]string{"error": code})
}
