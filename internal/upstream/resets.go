package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
)

// ConsumeResetCredit follows openai/codex backend-client's reset-credit contract.
// The caller must persist requestID before IO and reuse it for every replay.
func (c *Client) ConsumeResetCredit(ctx context.Context, credential accounts.Credential, requestID string) (string, error) {
	if credential.Kind() != "codex" {
		return "", ErrUsageUnsupported
	}
	if requestID == "" || len(requestID) > 128 || strings.ContainsAny(requestID, "\r\n") {
		return "", ErrInput
	}
	ctx = credentialContext(ctx, credential)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	body, _ := json.Marshal(struct {
		RequestID string `json:"redeem_request_id"`
	}{requestID})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits/consume", bytes.NewReader(body))
	if err != nil {
		return "", ErrInput
	}
	upstreamHeaders(request, credential, nil, false, c.codexVersion())
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return "", ErrUpstream
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", &UpstreamError{Status: response.StatusCode, RetryAfter: response.Header.Get("Retry-After")}
	}
	raw, err := readBounded(response.Body, 128<<10)
	if err != nil {
		return "", ErrResponse
	}
	var result struct {
		Code         string `json:"code"`
		WindowsReset int64  `json:"windows_reset"`
	}
	if json.Unmarshal(raw, &result) != nil || result.WindowsReset < 0 {
		return "", ErrResponse
	}
	switch result.Code {
	case "reset", "already_redeemed", "nothing_to_reset", "no_credit":
		return result.Code, nil
	default:
		return "", ErrResponse
	}
}
