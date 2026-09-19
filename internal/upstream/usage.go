package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
)

var ErrUsageUnsupported = errors.New("provider_usage_unsupported")

type Usage struct {
	Limits    []UsageLimit `json:"limits"`
	UpdatedAt int64        `json:"updated_at"`
}

type UsageLimit struct {
	Name         string        `json:"name"`
	Allowed      *bool         `json:"allowed"`
	LimitReached *bool         `json:"limit_reached"`
	Windows      []UsageWindow `json:"windows"`
}

type UsageWindow struct {
	Kind          string   `json:"kind"`
	UsedPercent   *float64 `json:"used_percent"`
	WindowSeconds *int64   `json:"window_seconds"`
	ResetAt       *int64   `json:"reset_at"`
}

type usageDetails struct {
	Allowed      *bool        `json:"allowed"`
	LimitReached *bool        `json:"limit_reached"`
	Primary      *usageWindow `json:"primary_window"`
	Secondary    *usageWindow `json:"secondary_window"`
}

type usageWindow struct {
	UsedPercent   *float64 `json:"used_percent"`
	WindowSeconds *int64   `json:"limit_window_seconds"`
	ResetAt       *int64   `json:"reset_at"`
	ResetAfter    *int64   `json:"reset_after_seconds"`
}

func (c *Client) Usage(ctx context.Context, credential accounts.Credential) (Usage, error) {
	if credential.Kind() != "codex" {
		return Usage{}, ErrUsageUnsupported
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// Match the ChatGPT backend path used by openai/codex's backend-client, independent of model forwarding.
	request, err := http.NewRequestWithContext(ctx, "GET", "https://chatgpt.com/backend-api/wham/usage", nil)
	if err != nil {
		return Usage{}, ErrInput
	}
	upstreamHeaders(request, credential, nil, false)
	response, err := c.http.Do(request)
	if err != nil {
		return Usage{}, ErrUpstream
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Usage{}, &UpstreamError{Status: response.StatusCode, RetryAfter: response.Header.Get("Retry-After")}
	}
	raw, err := readBounded(response.Body, 128<<10)
	if err != nil {
		return Usage{}, ErrResponse
	}
	var payload struct {
		RateLimit  json.RawMessage `json:"rate_limit"`
		Additional []struct {
			Name      string        `json:"limit_name"`
			RateLimit *usageDetails `json:"rate_limit"`
		} `json:"additional_rate_limits"`
	}
	if json.Unmarshal(raw, &payload) != nil || len(payload.RateLimit) == 0 || len(payload.Additional) > 32 {
		return Usage{}, ErrResponse
	}
	var primary *usageDetails
	if json.Unmarshal(payload.RateLimit, &primary) != nil {
		return Usage{}, ErrResponse
	}
	result := Usage{Limits: []UsageLimit{}, UpdatedAt: time.Now().Unix()}
	appendLimit := func(name string, details *usageDetails) error {
		if details == nil {
			return nil
		}
		if len(name) > 256 {
			return ErrResponse
		}
		limit := UsageLimit{Name: name, Allowed: details.Allowed, LimitReached: details.LimitReached, Windows: []UsageWindow{}}
		for i, value := range []*usageWindow{details.Primary, details.Secondary} {
			if value == nil {
				continue
			}
			if value.UsedPercent != nil && *value.UsedPercent < 0 ||
				value.WindowSeconds != nil && (*value.WindowSeconds <= 0 || *value.WindowSeconds > 366*24*3600) ||
				value.ResetAt != nil && (*value.ResetAt < 0 || *value.ResetAt > 253402300799) ||
				value.ResetAfter != nil && (*value.ResetAfter < 0 || *value.ResetAfter > 366*24*3600) {
				return ErrResponse
			}
			// Missing values stay unknown; a missing percentage must never imply a full quota.
			resetAt := value.ResetAt
			if resetAt == nil && value.ResetAfter != nil {
				reset := result.UpdatedAt + *value.ResetAfter
				resetAt = &reset
			}
			kind := "primary"
			if i == 1 {
				kind = "secondary"
			}
			limit.Windows = append(limit.Windows, UsageWindow{Kind: kind, UsedPercent: value.UsedPercent, WindowSeconds: value.WindowSeconds, ResetAt: resetAt})
		}
		result.Limits = append(result.Limits, limit)
		return nil
	}
	if err := appendLimit("", primary); err != nil {
		return Usage{}, err
	}
	for _, extra := range payload.Additional {
		if err := appendLimit(extra.Name, extra.RateLimit); err != nil {
			return Usage{}, err
		}
	}
	return result, nil
}
