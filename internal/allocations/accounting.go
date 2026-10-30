package allocations

import (
	"context"
	"encoding/json"

	"github.com/murongg/SubLane/internal/storage/db"
)

func memberLimit(config Config, member Share, window allocationWindow) int64 {
	if config.Mode == "ratio" {
		total := config.Total
		if config.Period == "durations" {
			total = window.limit
		}
		return ratioLimit(total, member.Limit)
	}
	if config.Mode == "windows" {
		for _, override := range member.WindowOverrides {
			if override.DurationSeconds == window.seconds {
				return override.Limit
			}
		}
		return window.limit
	}
	return member.Limit
}

func reservation(limit int64) int64 { return max(int64(1), (limit+9)/10) }

func (w allocationWindow) total(config Config) int64 {
	if config.Period == "durations" {
		return w.limit
	}
	return config.Total
}

func activeReservations(ctx context.Context, q *db.Queries, scheme, user int64, config Config, window allocationWindow) (int64, int64, error) {
	rows, err := q.AllocationActiveEntries(ctx, db.AllocationActiveEntriesParams{SchemeID: scheme, UserID: user, StartedFrom: window.start, StartedTo: window.end, Mode: entryMode(config)})
	if err != nil {
		return 0, 0, err
	}
	if len(rows) > 4096 {
		return 0, 0, ErrRisk
	}
	var held int64
	for _, row := range rows {
		var captured Config
		if err := json.Unmarshal([]byte(row.RevisionConfig), &captured); err != nil {
			return 0, 0, err
		}
		capturedWindow := window
		for _, condition := range captured.Windows {
			if condition.DurationSeconds == window.seconds {
				capturedWindow.limit = condition.Limit
			}
		}
		for _, member := range captured.Members {
			if member.UserID == row.UserID {
				limit := memberLimit(captured, member, capturedWindow)
				if captured.Mode != "windows" || limit != 0 {
					// Admission captured the cap; a later edit must not resize its lease.
					held += reservation(limit)
				}
				break
			}
		}
	}
	return int64(len(rows)), held, nil
}

func groupRoom(ctx context.Context, q *db.Queries, scheme int64, config Config, window allocationWindow) (int64, int64, int64, error) {
	used, err := q.AllocationGroupUsage(ctx, db.AllocationGroupUsageParams{SchemeID: scheme, StartedFrom: window.start, StartedTo: window.end, Mode: entryMode(config)})
	if err != nil {
		return 0, 0, 0, err
	}
	_, held, err := activeReservations(ctx, q, scheme, 0, config, window)
	limit := window.total(config)
	return used, held, max(0, limit+reservation(limit)-used-held), err
}
