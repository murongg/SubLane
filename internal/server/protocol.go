package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/murongg/SubLane/internal/gateway"
)

func nativeIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := gateway.WithRequestIdentity(r.Context(), 0, "http")
		w.Header().Set("Request-Id", gateway.RequestID(ctx))
		w.Header().Set("X-Request-Id", gateway.RequestID(ctx))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func gatewayWriters(kind gateway.Kind) (func(http.ResponseWriter, error), func(http.ResponseWriter, int, string)) {
	if kind == gateway.Messages {
		return messagesError, writeMessagesError
	}
	if kind.IsGemini() {
		return geminiError, writeGeminiError
	}
	return gatewayError, writeGatewayError
}

func gatewaySecret(r *http.Request, kind gateway.Kind) (string, bool) {
	if len(r.Header.Values("Authorization")) > 1 {
		return "", false
	}
	secret := ""
	if authorization := strings.TrimSpace(r.Header.Get("Authorization")); authorization != "" {
		scheme, token, ok := strings.Cut(authorization, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
			return "", false
		}
		secret = strings.TrimSpace(token)
	}
	var alternatives []string
	if kind == gateway.Messages {
		if len(r.Header.Values("X-Api-Key")) > 1 {
			return "", false
		}
		alternatives = append(alternatives, r.Header.Get("X-Api-Key"))
	} else if kind.IsGemini() {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(query["key"]) > 1 || len(r.Header.Values("X-Goog-Api-Key")) > 1 {
			return "", false
		}
		alternatives = append(alternatives, r.Header.Get("X-Goog-Api-Key"), query.Get("key"))
	}
	for _, value := range alternatives {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		// Never let a valid alternate credential rescue an invalid or conflicting Authorization header.
		if secret != "" && secret != value {
			return "", false
		}
		secret = value
	}
	return secret, secret != ""
}
