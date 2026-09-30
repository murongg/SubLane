package content

import (
	"context"
	"errors"
	"testing"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/tenants"
)

func TestWorkspaceIsolationAndAuditRollback(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	pattern := "SYNTHETIC_SECRET_ONE"
	badActor := audit.WithActor(ctx, audit.Actor{ID: 1, TenantID: 1, Username: "synthetic-owner", Role: "admin", Source: "invalid"})
	input := Input{Mode: "block", Rules: []RuleInput{{Name: "Synthetic one", Kind: "text", Pattern: &pattern, Enabled: true}}}
	if _, err := s.Update(badActor, input); !errors.Is(err, ErrUnavailable) {
		t.Fatal("audit failure accepted", err)
	}
	state, err := s.State(ctx)
	if err != nil || state.Mode != "off" || state.Revision != 0 {
		t.Fatal("audit failure published policy", state, err)
	}
	workspace, err := tenants.New(s.connection).Create(ctx, 1, "Synthetic second")
	if err != nil {
		t.Fatal(err)
	}
	other := New(s.connection, s.vault, workspace.ID)
	if _, err = s.Update(ctx, input); err != nil {
		t.Fatal(err)
	}
	result, err := other.Check(ctx, []byte(`{"input":"SYNTHETIC_SECRET_ONE"}`), 1<<20)
	if err != nil || result.Mode != "" || len(result.RuleIDs) != 0 {
		t.Fatal("cross-workspace rules applied", result, err)
	}
	actor := audit.WithActor(ctx, audit.Actor{ID: 1, TenantID: workspace.ID, Username: "synthetic-owner", Role: "admin", Source: "user"})
	pattern = "SYNTHETIC_SECRET_TWO"
	if _, err = other.Update(actor, input); err != nil {
		t.Fatal(err)
	}
	result, err = s.Check(ctx, []byte(`{"input":"SYNTHETIC_SECRET_TWO"}`), 1<<20)
	if err != nil || len(result.RuleIDs) != 0 {
		t.Fatal("second policy changed first workspace", result, err)
	}
	page, err := audit.NewForTenant(s.connection, workspace.ID).List(ctx, audit.Filter{Resource: "settings"})
	if err != nil || len(page.Events) != 1 {
		t.Fatal("workspace audit missing", page, err)
	}
}
