package gateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/groups"
)

func TestGroupModelPolicyEnforcesEveryOperationBeforeAdmission(t *testing.T) {
	calls := 0
	service, ids := discoveryGateway(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"models":[{"slug":"synthetic-allowed"},{"slug":"synthetic-denied"}]}`))}, nil
	}))
	ctx := context.Background()
	manager := groups.New(service.db)
	input := groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{ids["codex"]}, ModelPolicy: &groups.ModelPolicy{Restricted: true, Models: []string{"synthetic-allowed"}}}
	if _, err := manager.Save(ctx, 1, input); err != nil {
		t.Fatal(err)
	}
	models, err := service.Models(ctx, 1, 1)
	if err != nil || len(models) != 1 || models[0].ID != "synthetic-allowed" {
		t.Fatal("catalog policy", models, err)
	}
	before := calls
	for _, kind := range []Kind{Responses, Chat, Compact} {
		if _, err := service.Open(ctx, 1, 1, []byte(`{"model":"synthetic-denied","input":"synthetic"}`), http.Header{"Session_id": {"synthetic-session"}}, kind); !errors.Is(err, ErrModelNotAllowed) {
			t.Fatal("denied model forwarded", kind, err)
		}
	}
	if calls != before {
		t.Fatal("denied request reached upstream")
	}
	var bindings int
	if err := service.db.QueryRow("SELECT count(*) FROM account_affinity").Scan(&bindings); err != nil {
		t.Fatal(err)
	}
	if bindings != 0 {
		t.Fatal("denied request created affinity")
	}
	input.ModelPolicy.Models = []string{}
	if _, err := manager.Save(ctx, 1, input); err != nil {
		t.Fatal(err)
	}
	models, err = service.Models(ctx, 1, 1)
	if err != nil || len(models) != 0 || calls != before {
		t.Fatal("deny-all catalog reached upstream", err)
	}
	if _, err := service.Open(ctx, 1, 1, []byte(`{"model":"codex/synthetic-allowed"}`), nil, Responses); !errors.Is(err, ErrModelNotAllowed) {
		t.Fatal("changed policy ignored", err)
	}
}
