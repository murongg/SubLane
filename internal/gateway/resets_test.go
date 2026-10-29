package gateway

import (
	"context"
	"github.com/murongg/SubLane/internal/upstream"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestResetObservationUsesExplicitDenialAndRedemptionInvalidatesQuota(t *testing.T) {
	f := newQuotaFixture(t)
	ctx := context.Background()
	blocked := false
	used := "100"
	consumed := false
	client := upstream.NewWithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"available_count":2}`
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			allowed := "true"
			if blocked {
				allowed = "false"
			}
			body = `{"rate_limit":{"allowed":` + allowed + `,"primary_window":{"used_percent":` + used + `,"limit_window_seconds":18000,"reset_at":2000000000}}}`
		case "/backend-api/wham/rate-limit-reset-credits/consume":
			consumed = true
			blocked = false
			used = "0"
			body = `{"code":"reset","windows_reset":2}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	t.Cleanup(client.Close)
	f.service.provider = client
	observation, err := f.service.ResetObservation(ctx, f.id)
	if err != nil || !observation.Fresh || observation.Blocked {
		t.Fatal("display percentage caused redemption", observation, err)
	}
	blocked = true
	if err := f.service.queries.InvalidateAccountUsage(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	observation, err = f.service.ResetObservation(ctx, f.id)
	if err != nil || !observation.Blocked {
		t.Fatal(observation, err)
	}
	result, err := f.service.RedeemResetCredit(ctx, f.id, "synthetic-attempt")
	if err != nil || result != "reset" || !consumed {
		t.Fatal(result, err)
	}
	observation, err = f.service.ResetObservation(ctx, f.id)
	if err != nil || !observation.Fresh || observation.Blocked {
		t.Fatal("stale exhausted snapshot survived redemption", observation, err)
	}
	if _, err := f.accounts.SetEnabled(ctx, f.id, false); err != nil {
		t.Fatal(err)
	}
	consumed = false
	if _, err := f.service.RedeemResetCredit(ctx, f.id, "synthetic-next"); err == nil || consumed {
		t.Fatal("disabled account redeemed", err)
	}
}
