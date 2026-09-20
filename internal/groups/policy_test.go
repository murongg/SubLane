package groups_test

import (
	"context"
	"errors"
	"testing"

	"github.com/murongg/SubLane/internal/groups"
)

func TestModelPolicyDefaultsNormalizationAndPreservation(t *testing.T) {
	_, service, _, account := fixture(t)
	ctx := context.Background()
	detail, err := service.Get(ctx, 1)
	if err != nil || detail.RestrictedModels || len(detail.AllowedModels) != 0 {
		t.Fatal("default policy", err)
	}
	input := groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{account.ID}, ModelPolicy: &groups.ModelPolicy{Restricted: true, Models: []string{"synthetic-model", "claude/synthetic-model"}}}
	detail, err = service.Save(ctx, 1, input)
	if err != nil || !detail.RestrictedModels || len(detail.AllowedModels) != 2 {
		t.Fatal("policy save", err)
	}
	if detail.AllowedModels[1] != "codex/synthetic-model" {
		t.Fatal("missing canonical Codex prefix", detail.AllowedModels)
	}
	input.ModelPolicy = nil
	detail, err = service.Save(ctx, 1, input)
	if err != nil || !detail.RestrictedModels {
		t.Fatal("old client erased policy", err)
	}
	input.ModelPolicy = &groups.ModelPolicy{Restricted: true, Models: []string{"*"}}
	if _, err := service.Save(ctx, 1, input); !errors.Is(err, groups.ErrInput) {
		t.Fatal("wildcard accepted", err)
	}
	input.ModelPolicy = &groups.ModelPolicy{Restricted: true, Models: []string{}}
	detail, err = service.Save(ctx, 1, input)
	if err != nil || !detail.RestrictedModels || len(detail.AllowedModels) != 0 {
		t.Fatal("deny all unavailable", err)
	}
	input.ModelPolicy = &groups.ModelPolicy{}
	detail, err = service.Save(ctx, 1, input)
	if err != nil || detail.RestrictedModels {
		t.Fatal("unrestricted failed", err)
	}
}
