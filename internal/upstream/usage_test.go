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
		if r.URL.Path == "/backend-api/wham/rate-limit-reset-credits" {
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
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

func TestUsageReadsResetCreditsWithoutTreatingMissingAsZero(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want *int64
	}{
		{name: "available", raw: `{"rate_limit":null,"rate_limit_reset_credits":{"available_count":3}}`, want: ptrInt64(3)},
		{name: "zero", raw: `{"rate_limit":null,"rate_limit_reset_credits":{"available_count":0}}`, want: ptrInt64(0)},
		{name: "missing", raw: `{"rate_limit":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.raw))}, nil
			}))
			result, err := client.Usage(context.Background(), accounts.Credential{})
			if err != nil || (result.ResetCredits == nil) != (tc.want == nil) || result.ResetCredits != nil && *result.ResetCredits != *tc.want {
				t.Fatalf("reset credits: %v, %v", result.ResetCredits, err)
			}
		})
	}
}

func TestUsageKeepsQuotaWhenOptionalResetCreditsAreMalformed(t *testing.T) {
	for _, metadata := range []string{`"unexpected"`, `{"available_count":-1}`} {
		t.Run(metadata, func(t *testing.T) {
			client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/backend-api/wham/rate-limit-reset-credits" {
					return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"rate_limit":{"primary_window":{"used_percent":25,"limit_window_seconds":18000}},"rate_limit_reset_credits":` + metadata + `}`))}, nil
			}))
			result, err := client.Usage(context.Background(), accounts.Credential{})
			if err != nil || len(result.Limits) != 1 || len(result.Limits[0].Windows) != 1 || result.ResetCredits != nil {
				t.Fatalf("malformed optional metadata discarded quota: %+v, %v", result, err)
			}
		})
	}
}

func TestUsageReadsResetCreditsFromDetailsEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name       string
		usageCount string
		details    string
		status     int
		want       *int64
	}{
		{name: "details only", usageCount: ``, details: `{"available_count":2}`, status: 200, want: ptrInt64(2)},
		{name: "details override", usageCount: `,"rate_limit_reset_credits":{"available_count":1}`, details: `{"available_count":3}`, status: 200, want: ptrInt64(3)},
		{name: "details unavailable", usageCount: `,"rate_limit_reset_credits":{"available_count":1}`, status: 503, want: ptrInt64(1)},
		{name: "count missing", usageCount: ``, details: `{"credits":[]}`, status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
				paths = append(paths, r.URL.Path)
				if r.Header.Get("Authorization") != "Bearer synthetic-access" || r.Header.Get("Chatgpt-Account-Id") != "synthetic-account" {
					t.Fatal("reset-card request omitted account credentials")
				}
				switch r.URL.Path {
				case "/backend-api/wham/usage":
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"rate_limit":null` + tc.usageCount + `}`))}, nil
				case "/backend-api/wham/rate-limit-reset-credits":
					return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.details))}, nil
				default:
					t.Fatalf("unexpected path %s", r.URL.Path)
					return nil, nil
				}
			}))
			result, err := client.Usage(context.Background(), accounts.Credential{AccessToken: "synthetic-access", AccountID: "synthetic-account"})
			if err != nil || len(paths) != 2 || paths[0] != "/backend-api/wham/usage" || paths[1] != "/backend-api/wham/rate-limit-reset-credits" ||
				(result.ResetCredits == nil) != (tc.want == nil) || result.ResetCredits != nil && *result.ResetCredits != *tc.want {
				t.Fatalf("reset-card details: count=%v paths=%v err=%v", result.ResetCredits, paths, err)
			}
		})
	}
}

func ptrInt64(value int64) *int64 { return &value }

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
