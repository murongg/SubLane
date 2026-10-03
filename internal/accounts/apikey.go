package accounts

import (
	"crypto/rand"
	"encoding/json"
	"net/url"
	"strings"
	"unicode"
)

func parseAPIKey(raw []byte) (Credential, error) {
	var input struct {
		APIKey  string `json:"api_key"`
		BaseURL string `json:"base_url"`
	}
	if len(raw) == 0 || len(raw) > 65536 || json.Unmarshal(raw, &input) != nil || !validToken(input.APIKey) {
		return Credential{}, ErrInput
	}
	endpoint, err := normalizeBaseURL(input.BaseURL)
	if err != nil {
		return Credential{}, err
	}
	// API keys have no durable upstream subject. A separate identity survives rotation without reserving old keys.
	return Credential{Provider: "openai", BaseURL: endpoint, AccessToken: input.APIKey, AccountID: rand.Text()}, nil
}

func normalizeBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 2048 || strings.IndexFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "", ErrInput
	}
	u, err := url.Parse(value)
	if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", ErrInput
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "." || segment == ".." || strings.ContainsAny(segment, "\\\x00\r\n") {
			return "", ErrInput
		}
	}
	u.Host = strings.ToLower(u.Host)
	return strings.TrimRight(u.String(), "/"), nil
}
