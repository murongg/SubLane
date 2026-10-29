package resets

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/murongg/SubLane/internal/storage/db"
)

// Start owns two bounded workers across all workspaces, independent of browser/runtime activity.
func (s *Service) Start(parent context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	for range 2 {
		s.workers.Add(1)
		go s.work(ctx)
	}
}

func (s *Service) work(ctx context.Context) {
	defer s.workers.Done()
	for ctx.Err() == nil {
		now := s.now().Unix()
		row, err := s.queries.NextResetCheck(ctx, now)
		if err == nil {
			n, claimErr := s.queries.ClaimResetCheck(ctx, db.ClaimResetCheckParams{ID: row.ID, PreviousAt: row.ResetCheckAt, NextAt: now + 60})
			if claimErr == nil && n == 1 {
				attempt, cancel := context.WithTimeout(ctx, 90*time.Second)
				_, runErr := s.Run(attempt, row.TenantID, row.ID, "check")
				cancel()
				if runErr != nil && ctx.Err() == nil && !errors.Is(runErr, ErrBusy) {
					slog.Warn("reset credit check failed", slog.String("account_id", row.ID))
				}
				continue
			}
		} else if !errors.Is(err, sql.ErrNoRows) && ctx.Err() == nil {
			slog.Warn("reset credit scheduler unavailable")
		}
		timer := time.NewTimer(15 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.workers.Wait()
}
