package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/resets"
)

// ConsumeResetCredit follows openai/codex backend-client's reset-credit contract.
// The caller must persist requestID before IO and reuse it for every replay.
func (c *Client) ConsumeResetCredit(ctx context.Context, credential accounts.Credential, requestID, creditID string) (string, error) {
	if credential.Kind() != "codex" {
		return "", ErrUsageUnsupported
	}
	if requestID == "" || len(requestID) > 128 || strings.ContainsAny(requestID, "\r\n") {
		return "", ErrInput
	}
	if creditID != "" && !validCreditID(creditID) {
		return "", ErrInput
	}
	ctx = credentialContext(ctx, credential)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	body, _ := json.Marshal(struct {
		RequestID string `json:"redeem_request_id"`
		// Legacy pending attempts must replay their original omitted field; new attempts persist a selected ID.
		CreditID string `json:"credit_id,omitempty"`
	}{requestID, creditID})
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

func validCreditID(id string) bool {
	return id != "" && len(id) <= 256 && strings.TrimSpace(id) == id && strings.IndexFunc(id, unicode.IsControl) < 0
}

func (c *Client) resetCardPayload(ctx context.Context, credential accounts.Credential) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits", nil)
	if err != nil {
		return nil, ErrInput
	}
	upstreamHeaders(request, credential, nil, false, c.codexVersion())
	response, err := c.http.Do(request)
	if err != nil {
		return nil, ErrUpstream
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, &UpstreamError{Status: response.StatusCode, RetryAfter: response.Header.Get("Retry-After")}
	}
	raw, err := readBounded(response.Body, 128<<10)
	if err != nil {
		return nil, ErrResponse
	}
	return raw, nil
}

func (c *Client) ResetCards(ctx context.Context, credential accounts.Credential) (resets.CardList, error) {
	if credential.Kind() != "codex" {
		return resets.CardList{}, ErrUsageUnsupported
	}
	ctx, cancel := context.WithTimeout(credentialContext(ctx, credential), 20*time.Second)
	defer cancel()
	raw, err := c.resetCardPayload(ctx, credential)
	if err != nil {
		return resets.CardList{}, err
	}
	var payload struct {
		AvailableCount *int64 `json:"available_count"`
		Credits        *[]struct {
			ID        string  `json:"id"`
			Type      string  `json:"reset_type"`
			Status    string  `json:"status"`
			Title     string  `json:"title"`
			GrantedAt string  `json:"granted_at"`
			ExpiresAt *string `json:"expires_at"`
		} `json:"credits"`
	}
	if json.Unmarshal(raw, &payload) != nil || payload.Credits == nil || len(*payload.Credits) > 256 || (payload.AvailableCount != nil && *payload.AvailableCount < 0) {
		return resets.CardList{}, ErrResponse
	}
	result := resets.CardList{Cards: make([]resets.Card, 0, len(*payload.Credits)), AvailableCount: payload.AvailableCount, ObservedAt: time.Now().Unix()}
	seen := make(map[string]bool, len(*payload.Credits))
	for _, credit := range *payload.Credits {
		if !validCreditID(credit.ID) || seen[credit.ID] || len(credit.Title) > 256 {
			return resets.CardList{}, ErrResponse
		}
		seen[credit.ID] = true
		granted, err := time.Parse(time.RFC3339Nano, credit.GrantedAt)
		if err != nil || granted.Unix() < 0 {
			return resets.CardList{}, ErrResponse
		}
		card := resets.Card{ID: credit.ID, Title: credit.Title, Supported: credit.Type == "codex_rate_limits", Status: "unknown", GrantedAt: granted.Unix()}
		switch credit.Status {
		case "available", "redeeming", "redeemed":
			card.Status = credit.Status
		}
		if credit.ExpiresAt != nil {
			expiry, err := time.Parse(time.RFC3339Nano, *credit.ExpiresAt)
			if err != nil || expiry.Unix() < 0 {
				return resets.CardList{}, ErrResponse
			}
			at := expiry.Unix()
			card.ExpiresAt = &at
		}
		result.Cards = append(result.Cards, card)
	}
	return result, nil
}
