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
				if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["redeem_request_id"] != "synthetic-attempt-1" {
					t.Fatal("missing stable idempotency key or unnecessary provider fields")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":"` + code + `","windows_reset":2,"credit":{"id":"synthetic-private"}}`))}, nil
			}))
			got, err := client.ConsumeResetCredit(context.Background(), accounts.Credential{AccountID: "synthetic-subject", AccessToken: "synthetic-access"}, "synthetic-attempt-1")
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
		if _, err := client.ConsumeResetCredit(context.Background(), accounts.Credential{}, "synthetic-attempt"); !errors.Is(err, ErrResponse) {
			t.Fatalf("accepted ambiguous response: %v", err)
		}
	}
	client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsupported request reached provider")
		return nil, nil
	}))
	if _, err := client.ConsumeResetCredit(context.Background(), accounts.Credential{Provider: "claude"}, "synthetic-attempt"); !errors.Is(err, ErrUsageUnsupported) {
		t.Fatal(err)
	}
	if _, err := client.ConsumeResetCredit(context.Background(), accounts.Credential{}, ""); !errors.Is(err, ErrInput) {
		t.Fatal(err)
	}
}
