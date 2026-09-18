// Package codex isolates the pinned CLIProxyAPI translation SDK and the Codex upstream protocol.
package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/translator/builtin"
)

// Public OAuth and protocol constants match CLIProxyAPI v6.10.9's Codex adapter.
const (
	clientID      = "app_EMoamEEZ73f0CkXaXp7hrann"
	redirectURI   = "http://localhost:1455/auth/callback"
	clientVersion = "0.118.0"
	MaxBody       = 8 << 20
)

var (
	ErrInput        = errors.New("invalid_model_request")
	ErrContinuation = errors.New("continuation_requires_full_input")
	ErrUpstream     = errors.New("upstream_unavailable")
	ErrResponse     = errors.New("invalid_upstream_response")
)

type Client struct {
	http              *http.Client
	tokenURL, baseURL string
	registry          *translator.Registry
}

func New() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 20 * time.Second
	transport.MaxIdleConns = 16
	transport.MaxIdleConnsPerHost = 8
	return NewWithTransport(transport)
}

// NewWithTransport provides an HTTP seam for synthetic upstream tests. Production endpoints remain fixed.
func NewWithTransport(transport http.RoundTripper) *Client {
	return &Client{http: &http.Client{Transport: transport, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}, tokenURL: "https://auth.openai.com/oauth/token", baseURL: "https://chatgpt.com/backend-api/codex", registry: builtin.Registry()}
}

func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) AuthorizationURL(state, challenge string) string {
	query := url.Values{"client_id": {clientID}, "response_type": {"code"}, "redirect_uri": {redirectURI}, "scope": {"openid email profile offline_access"}, "state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "prompt": {"login"}, "id_token_add_organizations": {"true"}, "codex_cli_simplified_flow": {"true"}}
	return "https://auth.openai.com/oauth/authorize?" + query.Encode()
}

func (c *Client) Exchange(ctx context.Context, code, verifier string) (accounts.Credential, error) {
	return c.tokens(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {redirectURI}}, accounts.Credential{})
}

func (c *Client) Refresh(ctx context.Context, old accounts.Credential) (accounts.Credential, error) {
	return c.tokens(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {old.RefreshToken}}, old)
}

func (c *Client) tokens(ctx context.Context, form url.Values, old accounts.Credential) (accounts.Credential, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	form.Set("client_id", clientID)
	request, err := http.NewRequestWithContext(ctx, "POST", c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return accounts.Credential{}, ErrInput
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return accounts.Credential{}, ErrUpstream
	}
	defer response.Body.Close()
	if response.StatusCode == 400 || response.StatusCode == 401 {
		return accounts.Credential{}, accounts.ErrReauthorize
	}
	if response.StatusCode != 200 {
		return accounts.Credential{}, ErrUpstream
	}
	raw, err := readBounded(response.Body, 128<<10)
	if err != nil {
		return accounts.Credential{}, ErrResponse
	}
	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if json.Unmarshal(raw, &tokens) != nil || tokens.AccessToken == "" || tokens.ExpiresIn <= 0 || tokens.ExpiresIn > 366*24*3600 {
		return accounts.Credential{}, ErrResponse
	}
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = old.RefreshToken
	}
	if tokens.IDToken == "" {
		tokens.IDToken = old.IDToken
	}
	normalized, _ := json.Marshal(accounts.Credential{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken, IDToken: tokens.IDToken, AccountID: old.AccountID, Email: old.Email, Plan: old.Plan, ExpiresAt: time.Now().Unix() + tokens.ExpiresIn})
	credential, err := accounts.ParseCredential(normalized)
	if err != nil {
		return accounts.Credential{}, ErrResponse
	}
	return credential, nil
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

func (c *Client) Models(ctx context.Context, credential accounts.Credential) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/models?client_version="+clientVersion, nil)
	if err != nil {
		return nil, ErrInput
	}
	upstreamHeaders(request, credential, nil, false)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, ErrUpstream
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, &UpstreamError{Status: response.StatusCode, RetryAfter: response.Header.Get("Retry-After")}
	}
	raw, err := readBounded(response.Body, 2<<20)
	if err != nil {
		return nil, ErrResponse
	}
	var catalog struct {
		Models []struct {
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
		} `json:"models"`
	}
	if json.Unmarshal(raw, &catalog) != nil || catalog.Models == nil || len(catalog.Models) > 512 {
		return nil, ErrResponse
	}
	models := make([]Model, 0, len(catalog.Models))
	for _, model := range catalog.Models {
		if model.Slug != "" && len(model.Slug) <= 128 && model.Visibility != "hide" && model.Visibility != "hidden" {
			models = append(models, Model{ID: model.Slug, Object: "model", OwnedBy: "openai"})
		}
	}
	return models, nil
}

type UpstreamError struct {
	Status     int
	RetryAfter string
}

func (e *UpstreamError) Error() string { return "upstream_rejected_request" }

type Stream struct {
	*http.Response
	registry          *translator.Registry
	format            translator.Format
	model             string
	original, request []byte
	state             any
}

func (c *Client) Responses(ctx context.Context, credential accounts.Credential, raw []byte, headers http.Header, compact bool) (*Stream, error) {
	return c.execute(ctx, credential, raw, headers, translator.FormatOpenAIResponse, compact)
}

func (c *Client) Chat(ctx context.Context, credential accounts.Credential, raw []byte, headers http.Header) (*Stream, error) {
	return c.execute(ctx, credential, raw, headers, translator.FormatOpenAI, false)
}

func (c *Client) execute(ctx context.Context, credential accounts.Credential, raw []byte, headers http.Header, format translator.Format, compact bool) (*Stream, error) {
	var input map[string]json.RawMessage
	if len(raw) > MaxBody || json.Unmarshal(raw, &input) != nil || input == nil {
		return nil, ErrInput
	}
	var model, previous string
	if json.Unmarshal(input["model"], &model) != nil || model == "" || len(model) > 128 {
		return nil, ErrInput
	}
	if previousRaw := input["previous_response_id"]; len(previousRaw) > 0 && string(previousRaw) != "null" {
		if json.Unmarshal(previousRaw, &previous) != nil {
			return nil, ErrInput
		}
		// The Codex HTTP backend does not preserve Responses state. WebSocket callers must reconstruct full input first.
		if previous != "" {
			return nil, ErrContinuation
		}
	}
	translated := c.registry.TranslateRequest(format, translator.FormatCodex, model, raw, true)
	var body map[string]json.RawMessage
	if json.Unmarshal(translated, &body) != nil {
		return nil, ErrInput
	}
	body["model"], _ = json.Marshal(model)
	body["stream"] = json.RawMessage("true")
	body["store"] = json.RawMessage("false")
	if _, ok := body["instructions"]; !ok {
		body["instructions"] = json.RawMessage(`""`)
	}
	for _, key := range []string{"previous_response_id", "prompt_cache_retention", "safety_identifier", "stream_options"} {
		delete(body, key)
	}
	path := "/responses"
	if compact {
		path += "/compact"
		delete(body, "stream")
		delete(body, "store")
	}
	encoded, err := json.Marshal(body)
	if err != nil || len(encoded) > MaxBody {
		return nil, ErrInput
	}
	request, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, ErrInput
	}
	upstreamHeaders(request, credential, headers, !compact)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, ErrUpstream
	}
	return &Stream{Response: response, registry: c.registry, format: format, model: model, original: raw, request: encoded}, nil
}

func (s *Stream) Translate(ctx context.Context, event []byte) [][]byte {
	return s.registry.TranslateStream(ctx, translator.FormatCodex, s.format, s.model, s.original, s.request, append([]byte("data: "), event...), &s.state)
}

func (s *Stream) Complete(ctx context.Context, event []byte) []byte {
	var terminal struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(event, &terminal) == nil && terminal.Type == "response.incomplete" {
		var value map[string]json.RawMessage
		if json.Unmarshal(event, &value) != nil {
			return nil
		}
		// The SDK converter recognizes completed events; preserve the actual response.status and incomplete_details.
		value["type"] = json.RawMessage(`"response.completed"`)
		event, _ = json.Marshal(value)
	}
	return s.registry.TranslateNonStream(ctx, translator.FormatCodex, s.format, s.model, s.original, s.request, event, &s.state)
}

func upstreamHeaders(request *http.Request, credential accounts.Credential, client http.Header, stream bool) {
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	request.Header.Set("Chatgpt-Account-Id", credential.AccountID)
	request.Header.Set("Originator", "codex_cli_rs")
	request.Header.Set("User-Agent", "codex_cli_rs/"+clientVersion)
	request.Header.Set("Accept", "application/json")
	if stream {
		request.Header.Set("Accept", "text/event-stream")
	}
	// Client session metadata is useful; client credentials, cookies and arbitrary proxy headers are never forwarded.
	for _, key := range []string{"Version", "X-Codex-Turn-Metadata", "X-Client-Request-Id", "Session_id", "X-Codex-Beta-Features"} {
		if value := client.Get(key); len(value) > 0 && len(value) <= 1024 {
			request.Header.Set(key, value)
		}
	}
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrResponse
	}
	return raw, nil
}
