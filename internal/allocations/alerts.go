package allocations

import (
	"context"
	"fmt"
	"github.com/murongg/SubLane/internal/storage/db"
)

type ThresholdAlert struct {
	SchemeID, RevisionID, UserID, WindowSeconds, ResetAt, Threshold int64
	Cycle                                                           string
}

func (s *Service) ThresholdAlerts(ctx context.Context) ([]ThresholdAlert, error) {
	q := db.New(s.conn)
	rows, err := q.ListAllocationSchemes(ctx, s.tenantID)
	if err != nil {
		return nil, err
	}
	alerts := []ThresholdAlert{}
	for _, row := range rows {
		if row.Enabled == 0 {
			continue
		}
		detail, err := s.Detail(ctx, row.ID, 0)
		if err != nil {
			return nil, err
		}
		if !detail.Available {
			continue
		}
		origin, err := q.AllocationAnchorRevision(ctx, db.AllocationAnchorRevisionParams{SchemeID: row.ID, AccountingAnchorAt: detail.AccountingAnchorAt})
		if err != nil {
			return nil, err
		}
		for _, balance := range detail.Balances {
			if balance.Limit <= 0 {
				continue
			}
			allowed, err := q.CanUseAllocation(ctx, db.CanUseAllocationParams{ID: row.ID, UserID: balance.UserID})
			if err != nil {
				return nil, err
			}
			if !allowed {
				continue
			}
			for _, threshold := range []int64{80, 95} {
				// Compare integer units against a rounded-up threshold, avoiding overflow from used*100.
				if balance.Used >= (balance.Limit*threshold+99)/100 {
					if len(alerts) >= 4096 {
						return nil, ErrUnavailable
					}
					// Keep the legacy cycle format, using its anchor owner so edits cannot rearm it.
					alerts = append(alerts, ThresholdAlert{SchemeID: row.ID, RevisionID: detail.Revision.ID, UserID: balance.UserID, WindowSeconds: balance.WindowSeconds, ResetAt: balance.ResetAt, Threshold: threshold, Cycle: fmt.Sprintf("%d:%d", origin, balance.ResetAt)})
				}
			}
		}
	}
	return alerts, nil
}
