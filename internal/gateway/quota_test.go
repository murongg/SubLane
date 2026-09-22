package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/upstream"
)

func TestQuotaDecisionOnlyUsesFreshMainLimits(t *testing.T) {
	now := time.Now().Unix()
	for _, test := range []struct {
		name, raw string
		blocked   bool
	}{
		{"exhausted", `{"limits":[{"name":"","windows":[{"used_percent":100,"reset_at":RESET}]}]}`, true},
		{"remaining", `{"limits":[{"name":"","windows":[{"used_percent":99}]}]}`, false},
		{"explicit denial", `{"limits":[{"name":"","allowed":false}]}`, true},
		{"explicit reached", `{"limits":[{"name":"","limit_reached":true}]}`, true},
		{"additional limit", `{"limits":[{"name":"synthetic-special","allowed":false}]}`, false},
		{"unknown", `{"limits":[{"name":"","windows":[]}]}`, false},
		{"already reset", `{"limits":[{"name":"","allowed":false,"windows":[{"used_percent":100,"reset_at":PAST}]}]}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := stringsReplaceQuotaTime(test.raw, now)
			var usage upstream.Usage
			if err := json.Unmarshal([]byte(raw), &usage); err != nil {
				t.Fatal(err)
			}
			usage.UpdatedAt = now - 1
			got := quotaStatus(usage, time.Unix(now, 0))
			if (got.State == "exhausted") != test.blocked {
				t.Fatalf("unexpected decision: %+v", got)
			}
			usage.UpdatedAt = now - 121
			if quotaStatus(usage, time.Unix(now, 0)).State == "exhausted" {
				t.Fatal("stale snapshot blocked admission")
			}
			usage.UpdatedAt = now + 10
			if quotaStatus(usage, time.Unix(now, 0)).State == "exhausted" {
				t.Fatal("future snapshot blocked admission")
			}
		})
	}
}

func TestQuotaSelectionPersistsAndNeverMovesStickyConversation(t *testing.T) {
	ctx := context.Background()
	s, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	first, _, err := s.selectAccount(ctx, 1, 1, "synthetic-sticky", "codex", "", Responses)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.accounts.Authorize(ctx, "Synthetic second", accounts.Credential{AccountID: "quota-second", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("INSERT INTO group_accounts(group_id,account_id) VALUES(1,?)", second.ID); err != nil {
		t.Fatal(err)
	}
	saveExhaustedQuota(t, s, ids["codex"])
	restarted := New(ctx, s.db, s.accounts, s.provider)
	defer restarted.Close()
	chosen, _, err := restarted.selectAccount(ctx, 1, 1, "", "codex", "", Responses)
	if err != nil || chosen != second.ID {
		t.Fatal("exhausted account was selected", chosen, err)
	}
	chosen, _, err = restarted.selectAccount(ctx, 1, 1, "synthetic-sticky", "codex", "", Responses)
	if !errors.Is(err, ErrQuotaExhausted) || chosen != first {
		t.Fatal("sticky quota changed binding", chosen, err)
	}
	saveExhaustedQuota(t, s, second.ID)
	if _, _, err := restarted.selectAccount(ctx, 1, 1, "", "codex", "", Responses); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatal("pool quota not reported", err)
	}
	// Disabling and enabling invalidates observations from the older authorization lifecycle.
	if _, err := s.accounts.SetEnabled(ctx, first, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.accounts.SetEnabled(ctx, first, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := restarted.selectAccount(ctx, 1, 1, "synthetic-sticky", "codex", "", Responses); err != nil {
		t.Fatal("obsolete quota survived lifecycle change", err)
	}
}

func stringsReplaceQuotaTime(raw string, now int64) string {
	return strings.NewReplacer("RESET", strconv.FormatInt(now+60, 10), "PAST", strconv.FormatInt(now-1, 10)).Replace(raw)
}
func saveExhaustedQuota(t *testing.T, s *Service, id string) {
	t.Helper()
	now := s.now().Unix()
	raw := stringsReplaceQuotaTime(`{"updated_at":NOW,"limits":[{"name":"","limit_reached":true,"windows":[{"used_percent":100,"reset_at":RESET}]}]}`, now)
	raw = strings.ReplaceAll(raw, "NOW", strconv.FormatInt(now, 10))
	if _, err := s.db.Exec(`INSERT INTO account_usage(account_id,snapshot,updated_at,revision) SELECT id,?, ?, models_revision FROM accounts WHERE id=? ON CONFLICT(account_id) DO UPDATE SET snapshot=excluded.snapshot,updated_at=excluded.updated_at,revision=excluded.revision`, []byte(raw), now, id); err != nil {
		t.Fatal(err)
	}
}

func TestQuotaRejectionIsRecordedWithoutPenalizingAccount(t *testing.T) {
	s, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
		t.Error("exhausted account executed a model")
		return syntheticStream(), nil
	}))
	saveExhaustedQuota(t, s, ids["codex"])
	if _, err := s.Open(context.Background(), 1, 1, []byte(`{"model":"synthetic-model","input":"synthetic"}`), nil, Responses); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatal(err)
	}
	page, err := s.Requests(context.Background(), RequestFilter{})
	if err != nil || len(page.Requests) != 1 || page.Requests[0].ErrorCode != "quota_exhausted" || page.Requests[0].Outcome != "rejected" {
		t.Fatal("quota rejection missing", page, err)
	}
	states, err := s.Runtime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.ID == ids["codex"] && (state.State != "quota_exhausted" || state.Failures != 0 || state.InFlight != 0) {
			t.Fatal("quota conflated with transient failure", state)
		}
	}
}
