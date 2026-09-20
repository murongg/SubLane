package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/gateway"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/upstream"
)

type keyPrincipalKey struct{}

func (h *keyHTTP) registerGateway(router chi.Router) {
	routeErrors(router)
	router.Use(h.requireKey)
	router.Get("/models", h.models)
	router.Get("/responses", h.websocket)
	router.Post("/responses", func(w http.ResponseWriter, r *http.Request) { h.proxy(w, r, gateway.Responses) })
	router.Post("/responses/compact", func(w http.ResponseWriter, r *http.Request) { h.proxy(w, r, gateway.Compact) })
	router.Post("/chat/completions", func(w http.ResponseWriter, r *http.Request) { h.proxy(w, r, gateway.Chat) })
}

func (h *keyHTTP) requireKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme, secret, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			gatewayError(w, apikey.ErrInvalidKey)
			return
		}
		if !h.available(w) {
			return
		}
		principal, err := h.service.Authenticate(r.Context(), strings.TrimSpace(secret))
		if err != nil {
			gatewayError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), keyPrincipalKey{}, principal)))
	})
}

func (h *keyHTTP) models(w http.ResponseWriter, r *http.Request) {
	if h.gateway == nil {
		gatewayUnavailable(w, r)
		return
	}
	// Shared discovery owns upstream admission; a cached list consumes no generation slot.
	principal, _ := r.Context().Value(keyPrincipalKey{}).(apikey.Principal)
	models, err := h.gateway.Models(r.Context(), principal.UserID, principal.GroupID)
	if err != nil {
		gatewayError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"object": "list", "data": models})
}

func (h *keyHTTP) proxy(w http.ResponseWriter, r *http.Request, kind gateway.Kind) {
	if h.gateway == nil {
		gatewayUnavailable(w, r)
		return
	}
	release, err := h.gateway.Acquire()
	if err != nil {
		gatewayError(w, err)
		return
	}
	defer release()
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		writeGatewayError(w, 415, "json_required")
		return
	}
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(30 * time.Second))
	r.Body = http.MaxBytesReader(w, r.Body, upstream.MaxBody)
	raw, err := io.ReadAll(r.Body)
	_ = controller.SetReadDeadline(time.Time{})
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			writeGatewayError(w, 413, "request_too_large")
		} else {
			gatewayError(w, upstream.ErrInput)
		}
		return
	}
	var flags struct {
		Stream bool `json:"stream"`
	}
	if json.Unmarshal(raw, &flags) != nil || kind == gateway.Compact && flags.Stream {
		gatewayError(w, upstream.ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	principal, _ := r.Context().Value(keyPrincipalKey{}).(apikey.Principal)
	result, err := h.gateway.Open(gateway.WithRequestIdentity(ctx, principal.KeyID, "http"), principal.UserID, principal.GroupID, raw, r.Header, kind)
	if err != nil {
		gatewayError(w, err)
		return
	}
	defer result.Body.Close()
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		gatewayError(w, &upstream.UpstreamError{Status: result.StatusCode, RetryAfter: result.Header.Get("Retry-After")})
		return
	}
	if kind == gateway.Compact {
		data, err := io.ReadAll(io.LimitReader(result.Body, upstream.MaxBody+1))
		if err != nil || len(data) > upstream.MaxBody || !json.Valid(data) {
			result.Fail(upstream.ErrResponse)
			gatewayError(w, upstream.ErrResponse)
			return
		}
		result.AcceptCompact(data)
		writeRawJSON(w, data)
		return
	}
	started := false
	var completed []byte
	err = result.Events(func(event []byte) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !flags.Stream {
			completed = bytes.Clone(event)
			return nil
		}
		if !started {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Accel-Buffering", "no")
			w.WriteHeader(200)
			started = true
		}
		for _, chunk := range result.Translate(ctx, event) {
			chunk = bytes.TrimSpace(chunk)
			if len(chunk) == 0 {
				continue
			}
			if bytes.HasPrefix(chunk, []byte("data:")) {
				chunk = bytes.TrimSpace(chunk[5:])
			}
			if err := writeSSE(w, chunk, kind == gateway.Responses); err != nil {
				return err
			}
		}
		return controller.Flush()
	})
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		if !started {
			gatewayError(w, err)
			return
		}
		status, code := gatewayFailure(err)
		payload, _ := json.Marshal(map[string]any{"type": "error", "status": status, "error": gatewayErrorBody(code)})
		_ = writeSSE(w, payload, kind == gateway.Responses)
		_ = controller.Flush()
		return
	}
	if flags.Stream {
		if kind == gateway.Chat {
			_ = writeSSE(w, []byte("[DONE]"), false)
			_ = controller.Flush()
		}
		return
	}
	output := result.Complete(ctx, completed)
	if !json.Valid(output) {
		gatewayError(w, upstream.ErrResponse)
		return
	}
	writeRawJSON(w, output)
}

func writeRawJSON(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(data)
}

func writeSSE(w io.Writer, data []byte, named bool) error {
	if named {
		var event struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &event) == nil && event.Type != "" && len(event.Type) < 100 && strings.IndexFunc(event.Type, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) < 0 {
			if _, err := io.WriteString(w, "event: "+event.Type+"\n"); err != nil {
				return err
			}
		}
	}
	_, err := io.WriteString(w, "data: "+string(data)+"\n\n")
	return err
}

func gatewayFailure(err error) (int, string) {
	var rejected *upstream.UpstreamError
	switch {
	case errors.Is(err, apikey.ErrInvalidKey):
		return 401, "invalid_api_key"
	case errors.Is(err, gateway.ErrContextLimit):
		return 400, "conversation_context_limit"
	case errors.Is(err, gateway.ErrModelNotAllowed):
		return 403, "model_not_allowed"
	case errors.Is(err, gateway.ErrModelUnavailable):
		return 404, "model_not_available"
	case errors.Is(err, gateway.ErrCatalogUnavailable):
		return 503, "model_catalog_unavailable"
	case errors.Is(err, upstream.ErrInput):
		return 400, "invalid_model_request"
	case errors.Is(err, upstream.ErrContinuation):
		return 400, "continuation_requires_full_input"
	case errors.Is(err, groups.ErrUnavailable):
		return 403, "group_unavailable"
	case errors.Is(err, gateway.ErrAffinityUnavailable):
		return 409, "conversation_account_unavailable"
	case errors.Is(err, gateway.ErrNoAccount):
		return 503, "no_accounts_available"
	case errors.Is(err, gateway.ErrAccountCooling):
		return 429, "account_cooling"
	case errors.Is(err, gateway.ErrMemberBusy):
		return 429, "member_busy"
	case errors.Is(err, gateway.ErrMemberRate):
		return 429, "member_rate_limited"
	case errors.Is(err, gateway.ErrAccountBusy):
		return 429, "account_busy"
	case errors.Is(err, gateway.ErrBusy), errors.Is(err, gateway.ErrAffinityLimit):
		return 429, "gateway_busy"
	case errors.Is(err, accounts.ErrDisabled), errors.Is(err, accounts.ErrNotFound):
		return 503, "account_unavailable"
	case errors.Is(err, accounts.ErrReauthorize):
		return 503, "account_reauthorization_required"
	case errors.Is(err, context.DeadlineExceeded):
		return 504, "gateway_timeout"
	case errors.Is(err, upstream.ErrInterrupted):
		return 502, "upstream_stream_interrupted"
	case errors.As(err, &rejected):
		switch rejected.Status {
		case 400:
			return 400, "upstream_invalid_request"
		case 404:
			return 404, "model_not_found"
		case 401, 403:
			return 503, "account_reauthorization_required"
		case 429:
			return 429, "upstream_rate_limited"
		default:
			return 502, "upstream_unavailable"
		}
	default:
		return 502, "upstream_unavailable"
	}
}

func gatewayError(w http.ResponseWriter, err error) {
	status, code := gatewayFailure(err)
	if errors.Is(err, gateway.ErrCatalogUnavailable) {
		w.Header().Set("Retry-After", "5")
	}
	if status == 401 {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	if status == 429 {
		value := "1"
		var rejected *upstream.UpstreamError
		if errors.As(err, &rejected) {
			value = rejected.RetryAfter
		}
		var cooling *gateway.CoolingError
		if errors.As(err, &cooling) {
			value = strconv.FormatInt(cooling.RetryAfter, 10)
		}
		var memberRate *gateway.MemberRateError
		if errors.As(err, &memberRate) {
			value = strconv.FormatInt(memberRate.RetryAfter, 10)
		}
		retryAfter(w, value)
	}
	writeGatewayError(w, status, code)
}

func gatewayErrorBody(code string) map[string]string {
	return map[string]string{"type": "gateway_error", "code": code, "message": strings.ReplaceAll(code, "_", " ")}
}

func writeGatewayError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": gatewayErrorBody(code)})
}

func gatewayUnavailable(w http.ResponseWriter, r *http.Request) {
	writeGatewayError(w, 501, "gateway_not_configured")
}
