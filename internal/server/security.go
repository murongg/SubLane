package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/auth"
)

const challengeCookie = "sublane_auth_challenge"

func (h *authHTTP) registerSecurity(router chi.Router) {
	router.Get("/security", h.securityState)
	router.Group(func(r chi.Router) {
		r.Use(h.throttleEnrollment)
		r.Post("/totp", h.beginTOTP)
		r.Post("/totp/confirm", h.confirmTOTP)
		r.Delete("/totp", h.disableTOTP)
		r.Post("/recovery-codes", h.regenerateRecoveryCodes)
		r.Post("/passkeys/begin", h.beginPasskeyRegistration)
		r.Post("/passkeys/finish", h.finishPasskeyRegistration)
		r.Delete("/passkeys/{id}", h.deletePasskey)
	})
}
func (h *authHTTP) challengeToken(r *http.Request) string {
	cookie, err := r.Cookie(challengeCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}
func (h *authHTTP) setChallenge(w http.ResponseWriter, r *http.Request, token string) {
	age := 300
	expires := time.Now().Add(5 * time.Minute)
	if token == "" {
		age = -1
		expires = time.Unix(1, 0)
	}
	http.SetCookie(w, &http.Cookie{Name: challengeCookie, Value: token, Path: "/api", HttpOnly: true, Secure: h.secure(r), SameSite: http.SameSiteStrictMode, MaxAge: age, Expires: expires})
}
func (h *authHTTP) origin(r *http.Request) string {
	if h.publicURL != "" {
		return h.publicURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
func (h *authHTTP) securityState(w http.ResponseWriter, r *http.Request) {
	state, err := h.service.SecurityState(r.Context(), token(r))
	if err != nil {
		authError(w, err)
		return
	}
	writeJSON(w, 200, state)
}
func (h *authHTTP) beginTOTP(w http.ResponseWriter, r *http.Request) {
	var proof auth.Proof
	if !decodeJSON(w, r, &proof) {
		return
	}
	setup, err := h.service.BeginTOTP(r.Context(), token(r), proof)
	if err != nil {
		authError(w, err)
		return
	}
	h.setChallenge(w, r, setup.Challenge)
	writeJSON(w, 200, setup)
}
func (h *authHTTP) confirmTOTP(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	codes, err := h.service.EnableTOTP(r.Context(), token(r), h.challengeToken(r), input.Code)
	if err != nil {
		authError(w, err)
		return
	}
	h.setChallenge(w, r, "")
	writeJSON(w, 200, map[string]any{"recovery_codes": codes})
}
func (h *authHTTP) disableTOTP(w http.ResponseWriter, r *http.Request) {
	var proof auth.Proof
	if !decodeJSON(w, r, &proof) {
		return
	}
	if err := h.service.DisableTOTP(r.Context(), token(r), proof); err != nil {
		authError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *authHTTP) regenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	var proof auth.Proof
	if !decodeJSON(w, r, &proof) {
		return
	}
	codes, err := h.service.RegenerateRecoveryCodes(r.Context(), token(r), proof)
	if err != nil {
		authError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"recovery_codes": codes})
}
func (h *authHTTP) beginPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	var proof auth.Proof
	if !decodeJSON(w, r, &proof) {
		return
	}
	options, challenge, err := h.service.BeginPasskeyRegistration(r.Context(), token(r), h.origin(r), proof)
	if err != nil {
		authError(w, err)
		return
	}
	h.setChallenge(w, r, challenge)
	writeJSON(w, 200, options)
}
func (h *authHTTP) finishPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name       string          `json:"name"`
		Credential json.RawMessage `json:"credential"`
	}
	if !decodeJSONLimit(w, r, &input, 65536) {
		return
	}
	challenge := h.challengeToken(r)
	h.setChallenge(w, r, "")
	if err := h.service.FinishPasskeyRegistration(r.Context(), token(r), challenge, h.origin(r), input.Name, input.Credential); err != nil {
		authError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *authHTTP) deletePasskey(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		authError(w, auth.ErrInput)
		return
	}
	var proof auth.Proof
	if !decodeJSON(w, r, &proof) {
		return
	}
	if err := h.service.DeletePasskey(r.Context(), token(r), id, proof); err != nil {
		authError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *authHTTP) beginPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decodeJSON(w, r, &input) {
		return
	}
	options, challenge, err := h.service.BeginPasskeyLogin(r.Context(), h.origin(r))
	if err != nil {
		authError(w, err)
		return
	}
	h.setChallenge(w, r, challenge)
	writeJSON(w, 200, options)
}
func (h *authHTTP) finishPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Credential json.RawMessage `json:"credential"`
	}
	if !decodeJSONLimit(w, r, &input, 65536) {
		return
	}
	challenge := h.challengeToken(r)
	h.setChallenge(w, r, "")
	session, err := h.service.FinishPasskeyLogin(r.Context(), challenge, h.origin(r), input.Credential)
	if err != nil {
		authError(w, err)
		return
	}
	h.completeSession(w, r, session, 200)
}
