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
	if detail.AllowedModels[1] != "synthetic-model" {
		t.Fatal("native policy changed unexpectedly", detail.AllowedModels)
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

func TestNativePoliciesMatchEachProviderWithoutBroadeningLegacyRules(t *testing.T) {
	native := groups.ModelPolicy{Restricted: true, Models: []string{"synthetic-model"}}
	legacy := groups.ModelPolicy{Restricted: true, Models: []string{"claude/synthetic-model"}}
	for _, model := range []string{"synthetic-model", "codex/synthetic-model", "claude/synthetic-model", "antigravity/synthetic-model"} {
		if !native.Allows(model) {
			t.Fatal("native rule did not match", model)
		}
	}
	if !legacy.Allows("synthetic-model") || !legacy.Allows("claude/synthetic-model") || legacy.Allows("codex/synthetic-model") || legacy.Allows("antigravity/synthetic-model") || legacy.Allows("synthetic-other") {
		t.Fatal("legacy provider scope was widened or became unusable")
	}
}
