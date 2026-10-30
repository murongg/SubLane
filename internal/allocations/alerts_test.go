package allocations

import (
	"context"
	"testing"
)

func TestAllowanceThresholdCycleSurvivesMemberRevision(t *testing.T) {
	s, conn, user, _ := fixture(t)
	ctx := context.Background()
	now := syntheticClock(s)
	in := SchemeInput{Name: "Synthetic stable alert", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}}
	out, err := s.SaveScheme(ctx, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO allocation_entries(request_id,scheme_id,revision_id,user_id,account_id,model,mode,state,started_at,reset_at,window_start,cost) VALUES('synthetic-alert',?,?,?,'synthetic-account','synthetic','tokens','settled',?,?,?,90)`, out.ID, out.Revision.ID, user, *now, *now+86400, *now); err != nil {
		t.Fatal(err)
	}
	a, err := s.ThresholdAlerts(ctx)
	if err != nil || len(a) == 0 {
		t.Fatal(a, err)
	}
	in.ApplyNow = true
	in.Config.Members = []Share{{UserID: user, Limit: 101}}
	if _, err := s.SaveScheme(ctx, out.ID, conditioned(t, s, out.ID, in)); err != nil {
		t.Fatal(err)
	}
	b, err := s.ThresholdAlerts(ctx)
	if err != nil || len(b) == 0 || a[0].Cycle != b[0].Cycle {
		t.Fatal("member edit rearmed the period", a, b, err)
	}
}

func TestThresholdAlertsUseSettledUsageAndCurrentWindow(t *testing.T) {
	s, conn, user, _ := fixture(t)
	ctx := context.Background()
	scheme, err := s.SaveScheme(ctx, 0, SchemeInput{Name: "Synthetic allowance", GroupID: 2, Enabled: true, Config: Config{Mode: "tokens", Period: "day", Members: []Share{{UserID: user, Limit: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	// Use the accounting table's settled entries, never inferred pending exposure.
	if _, err := conn.Exec(`INSERT INTO allocation_entries(request_id,scheme_id,revision_id,user_id,account_id,model,mode,state,started_at,reset_at,window_start,input_tokens,cost) VALUES('synthetic-threshold',?,?,?,'synthetic-account','synthetic-model','tokens','settled',?,?,?,80,80)`, scheme.ID, scheme.Revision.ID, user, s.now().Unix(), s.now().Unix()+86400, s.now().Unix()); err != nil {
		t.Fatal(err)
	}
	alerts, err := s.ThresholdAlerts(ctx)
	if err != nil || len(alerts) != 1 || alerts[0].Threshold != 80 || alerts[0].UserID != user {
		t.Fatal(alerts, err)
	}
	if _, err := conn.Exec("UPDATE allocation_entries SET input_tokens=95,cost=95 WHERE request_id='synthetic-threshold'"); err != nil {
		t.Fatal(err)
	}
	alerts, err = s.ThresholdAlerts(ctx)
	if err != nil || len(alerts) != 2 {
		t.Fatal(alerts, err)
	}
}
