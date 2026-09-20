package gateway

import (
	"context"
	"errors"

	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/upstream"
)

var ErrModelNotAllowed = errors.New("model_not_allowed")

func (s *Service) modelPolicy(ctx context.Context, userID, groupID int64) (groups.ModelPolicy, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return groups.ModelPolicy{}, err
	}
	defer tx.Rollback()
	policy, err := groups.ReadPolicy(ctx, s.queries.WithTx(tx), userID, groupID)
	if err != nil {
		return policy, err
	}
	return policy, tx.Commit()
}
func policyHasProvider(policy groups.ModelPolicy, provider string) bool {
	if !policy.Restricted {
		return true
	}
	for _, model := range policy.Models {
		scope, _ := groups.SplitModel(model)
		if scope == "" || scope == provider {
			return true
		}
	}
	return false
}

// AuthorizeModel also covers local WebSocket prewarm, which does not execute an upstream request.
func (s *Service) AuthorizeModel(ctx context.Context, userID, groupID int64, model string) error {
	_, native := groups.SplitModel(model)
	if native == "" || len(native) > 128 {
		return upstream.ErrInput
	}
	policy, err := s.modelPolicy(ctx, userID, groupID)
	if err != nil {
		return err
	}
	if !policy.Allows(model) {
		return ErrModelNotAllowed
	}
	return nil
}
