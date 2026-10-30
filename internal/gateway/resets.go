package gateway

import (
	"context"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/resets"
	"github.com/murongg/SubLane/internal/storage/db"
)

func (s *Service) ResetObservation(ctx context.Context, id string) (resets.Observation, error) {
	account, err := s.accounts.Get(ctx, id)
	if err != nil {
		return resets.Observation{}, err
	}
	if account.Provider != "codex" {
		return resets.Observation{}, resets.ErrUnsupported
	}
	snapshot, err := s.RefreshUsage(ctx, id)
	if err != nil {
		return resets.Observation{}, err
	}
	value := resets.Observation{Fresh: !snapshot.Stale && !snapshot.RefreshFailed && !snapshot.Refreshing, Credits: snapshot.ResetCredits}
	for _, limit := range snapshot.Limits {
		if limit.Name != "" {
			continue
		}
		// Ordinary-usage permission is authoritative; rounded percentages are not redemption eligibility.
		if limit.Allowed != nil {
			value.Blocked = !*limit.Allowed
		} else {
			value.Blocked = limit.LimitReached != nil && *limit.LimitReached
		}
	}
	if value.Fresh && value.Credits != nil && *value.Credits > 0 {
		cards, err := s.ResetCards(ctx, id)
		if err == nil {
			value.Cards = cards.Cards
		}
	}
	return value, nil
}

func (s *Service) ResetCards(ctx context.Context, id string) (resets.CardList, error) {
	account, err := s.accounts.Get(ctx, id)
	if err != nil {
		return resets.CardList{}, err
	}
	if account.Provider != "codex" {
		return resets.CardList{}, resets.ErrUnsupported
	}
	if err := s.usageAccount(ctx, id); err != nil {
		return resets.CardList{}, err
	}
	before, err := s.queries.GetResetAccount(ctx, db.GetResetAccountParams{ID: id, TenantID: s.tenantID})
	if err != nil {
		return resets.CardList{}, err
	}
	release, err := s.acquireUsage()
	if err != nil {
		return resets.CardList{}, err
	}
	defer release()
	value, err := readAccount(ctx, s, id, s.provider.ResetCards)
	if err != nil {
		return resets.CardList{}, err
	}
	if err := s.usageAccount(ctx, id); err != nil {
		return resets.CardList{}, err
	}
	after, err := s.queries.GetResetAccount(ctx, db.GetResetAccountParams{ID: id, TenantID: s.tenantID})
	if err != nil {
		return resets.CardList{}, err
	}
	if before.ModelsRevision != after.ModelsRevision {
		return resets.CardList{}, errUsageChanged
	}
	return resets.PrepareCards(value, s.now()), nil
}

func (s *Service) RedeemResetCredit(ctx context.Context, id, requestID, creditID string) (string, error) {
	account, err := s.accounts.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if account.Provider != "codex" {
		return "", resets.ErrUnsupported
	}
	if err := s.usageAccount(ctx, id); err != nil {
		return "", err
	}
	release, err := s.acquireUsage()
	if err != nil {
		return "", err
	}
	result, err := readAccount(ctx, s, id, func(ctx context.Context, c accounts.Credential) (string, error) {
		return s.provider.ConsumeResetCredit(ctx, c, requestID, creditID)
	})
	release()
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	// Fence polls and inference observations started before redemption, including ambiguous network outcomes.
	if invalidateErr := s.queries.InvalidateAccountUsage(writeCtx, id); invalidateErr != nil {
		return "", invalidateErr
	}
	if err == nil {
		// A failed follow-up read cannot turn a confirmed redemption into a second spend.
		_, _ = s.RefreshUsage(ctx, id)
	}
	return result, err
}
