package upstream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/accounts"
)

type usageTransport func(*http.Request) (*http.Response, error)

func (f usageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUsageReadsActualWindowsWithoutExposingProviderMetadata(t *testing.T) {
	client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.String() != "https://chatgpt.com/backend-api/wham/usage" || r.Header.Get("Authorization") != "Bearer synthetic-access" || r.Header.Get("Chatgpt-Account-Id") != "synthetic-account" {
			t.Fatal("incorrect usage request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"rate_limit":{"allowed":false,"limit_reached":true,"primary_window":{"used_percent":102,"limit_window_seconds":18000,"reset_at":2000000000},"secondary_window":{"used_percent":12.5,"limit_window_seconds":604800,"reset_after_seconds":120}},"additional_rate_limits":[{"limit_name":"Synthetic model","metered_feature":"synthetic-model","rate_limit":{"primary_window":{"limit_window_seconds":3600}}}],"account_id":"private-account"}`))}, nil
	}))
	before := int64(0)
	result, err := client.Usage(context.Background(), accounts.Credential{AccessToken: "synthetic-access", AccountID: "synthetic-account"})
	if err != nil || len(result.Limits) != 2 || result.UpdatedAt <= before {
		t.Fatalf("usage: %+v, %v", result, err)
	}
	primary, secondary := result.Limits[0].Windows[0], result.Limits[0].Windows[1]
	if *primary.UsedPercent != 102 || *primary.WindowSeconds != 18000 || *primary.ResetAt != 2000000000 || *secondary.ResetAt != result.UpdatedAt+120 || *secondary.UsedPercent != 12.5 {
		t.Fatal("upstream windows were changed or reset time lost")
	}
	if result.Limits[0].Allowed == nil || *result.Limits[0].Allowed || result.Limits[1].Windows[0].UsedPercent != nil || result.Limits[1].Windows[0].ResetAt != nil {
		t.Fatal("missing fields became fabricated usage")
	}
}

func TestUsageRejectsMalformedResponsesAndKeepsMissingLimitsUnknown(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `{"rate_limit":{"primary_window":{"used_percent":-1}}}`, `{"rate_limit":{"primary_window":{"limit_window_seconds":0}}}`, `{"rate_limit":{"primary_window":{"reset_at":-1}}}`, `{"rate_limit":{"primary_window":{"used_percent":"42"}}}`} {
		t.Run(raw, func(t *testing.T) {
			client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw))}, nil
			}))
			if _, err := client.Usage(context.Background(), accounts.Credential{}); !errors.Is(err, ErrResponse) {
				t.Fatalf("accepted invalid usage: %v", err)
			}
		})
	}
	client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"rate_limit":null}`))}, nil
	}))
	result, err := client.Usage(context.Background(), accounts.Credential{})
	if err != nil || len(result.Limits) != 0 || result.Limits == nil {
		t.Fatal("missing quota must be an empty list, not zero usage", err)
	}
}

func TestUsagePreservesSafeUpstreamErrors(t *testing.T) {
	client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"30"}}, Body: io.NopCloser(strings.NewReader(`{"secret":"synthetic-private"}`))}, nil
	}))
	_, err := client.Usage(context.Background(), accounts.Credential{})
	var rejected *UpstreamError
	if !errors.As(err, &rejected) || rejected.Status != 429 || rejected.RetryAfter != "30" || strings.Contains(err.Error(), "synthetic-private") {
		t.Fatal("unsafe or missing upstream error", err)
	}
}
