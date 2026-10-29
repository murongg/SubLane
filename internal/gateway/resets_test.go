package gateway

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/upstream"
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

func TestResetQuotaRevisionReachesBatchedAdmissionAndRuntime(t *testing.T) {
	f := newQuotaFixture(t)
	ctx := context.Background()
	if err := f.service.queries.InvalidateAccountUsage(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RefreshUsage(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	workspace, err := f.service.queries.ListWorkspaceUsage(ctx, 1)
	if err != nil || len(workspace) != 1 || len(workspace[0].Snapshot) == 0 {
		t.Fatal("workspace quota lost after reset", workspace, err)
	}
	group, err := f.service.queries.ListGroupUsage(ctx, 1)
	if err != nil || len(group) != 1 || len(group[0].Snapshot) == 0 {
		t.Fatal("group quota lost after reset", group, err)
	}
	routing, err := f.service.queries.ListRoutingAccounts(ctx, db.ListRoutingAccountsParams{TenantID: 1, GroupID: 1})
	if err != nil || len(routing) != 1 || len(routing[0].UsageSnapshot) == 0 {
		t.Fatal("admission quota lost after reset", routing, err)
	}
	states, err := f.service.Runtime(ctx)
	if err != nil || len(states) != 1 || states[0].QuotaState != "available" {
		t.Fatal("runtime discarded post-reset quota", states, err)
	}
}
