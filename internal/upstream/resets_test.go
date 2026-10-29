package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/accounts"
)

func TestConsumeResetCreditUsesStableRequestIDAndRecognizedOutcomes(t *testing.T) {
	for _, code := range []string{"reset", "already_redeemed", "nothing_to_reset", "no_credit"} {
		t.Run(code, func(t *testing.T) {
			client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost || r.URL.String() != "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits/consume" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "Bearer synthetic-access" || r.Header.Get("Chatgpt-Account-Id") != "synthetic-subject" {
					t.Fatal("wrong reset request")
				}
				var body map[string]string
				if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 2 || body["redeem_request_id"] != "synthetic-attempt-1" || body["credit_id"] != "synthetic-selected-card" {
					t.Fatal("missing stable idempotency key or unnecessary provider fields")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":"` + code + `","windows_reset":2,"credit":{"id":"synthetic-private"}}`))}, nil
			}))
			got, err := client.ConsumeResetCredit(context.Background(), accounts.Credential{AccountID: "synthetic-subject", AccessToken: "synthetic-access"}, "synthetic-attempt-1", "synthetic-selected-card")
			if err != nil || got != code {
				t.Fatalf("outcome %q, %v", got, err)
			}
		})
	}
}

func TestConsumeResetCreditRejectsUnknownResponsesAndUnsupportedProviders(t *testing.T) {
	for _, body := range []string{`{}`, `{"code":"future_outcome"}`, `{"code":"reset","windows_reset":-1}`, strings.Repeat("x", 129<<10)} {
		client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		}))
		if _, err := client.ConsumeResetCredit(context.Background(), accounts.Credential{}, "synthetic-attempt", ""); !errors.Is(err, ErrResponse) {
			t.Fatalf("accepted ambiguous response: %v", err)
		}
	}
	client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsupported request reached provider")
		return nil, nil
	}))
	if _, err := client.ConsumeResetCredit(context.Background(), accounts.Credential{Provider: "claude"}, "synthetic-attempt", ""); !errors.Is(err, ErrUsageUnsupported) {
		t.Fatal(err)
	}
	if _, err := client.ConsumeResetCredit(context.Background(), accounts.Credential{}, "", ""); !errors.Is(err, ErrInput) {
		t.Fatal(err)
	}
}

func TestLegacyConsumeOmitsCardID(t *testing.T) {
	client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["redeem_request_id"] != "synthetic-legacy" {
			t.Fatal(body)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":"already_redeemed"}`))}, nil
	}))
	if _, err := client.ConsumeResetCredit(context.Background(), accounts.Credential{}, "synthetic-legacy", ""); err != nil {
		t.Fatal(err)
	}
}

func TestResetCardsReadsNormalizedMetadataWithoutProviderProfiles(t *testing.T) {
	client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.String() != "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits" || r.Header.Get("Authorization") != "Bearer synthetic-access" {
			t.Fatal("incorrect card read")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"available_count":2,"credits":[{"id":"synthetic-first","reset_type":"codex_rate_limits","status":"available","granted_at":"2026-01-01T00:00:00Z","expires_at":"2030-01-01T00:00:00Z","title":"Synthetic full reset","profile_user_id":"synthetic-private-profile","profile_image_url":"https://example.test/private.png"},{"id":"synthetic-future","reset_type":"future_type","status":"future_status","granted_at":"2026-01-02T00:00:00Z","expires_at":null}]}`))}, nil
	}))
	got, err := client.ResetCards(context.Background(), accounts.Credential{AccessToken: "synthetic-access"})
	if err != nil || len(got.Cards) != 2 || got.AvailableCount == nil || *got.AvailableCount != 2 || got.ObservedAt <= 0 {
		t.Fatal(got, err)
	}
	if !got.Cards[0].Supported || got.Cards[0].ExpiresAt == nil || got.Cards[1].Supported || got.Cards[1].Status != "unknown" {
		t.Fatal(got)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "profile") {
		t.Fatal("provider profile leaked")
	}
}

func TestResetCardsRejectsAmbiguousInventory(t *testing.T) {
	for _, body := range []string{`{"available_count":1}`, `{"credits":[{"id":"synthetic","status":"available","reset_type":"codex_rate_limits","granted_at":"2026-01-01T00:00:00Z","expires_at":"not-a-date"}]}`, `{"available_count":-1,"credits":[]}`, `{"credits":[{"id":"synthetic","granted_at":"2026-01-01T00:00:00Z"},{"id":"synthetic","granted_at":"2026-01-01T00:00:00Z"}]}`, strings.Repeat("x", 129<<10)} {
		client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		}))
		if _, err := client.ResetCards(context.Background(), accounts.Credential{}); !errors.Is(err, ErrResponse) {
			t.Fatal("unsafe card inventory accepted", err)
		}
	}
}
