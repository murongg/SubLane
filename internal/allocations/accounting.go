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

type sharedUsage struct{ used, held, room int64 }

// Each calculation owns its snapshot; a later admission or report must see newly settled requests.
type accounting struct {
	q        *db.Queries
	scheme   int64
	config   Config
	windows  []allocationWindow
	active   map[int64][]db.AllocationActiveEntriesRow
	captured map[int64]Config
	groups   map[allocationWindow]sharedUsage
}

func newAccounting(q *db.Queries, scheme int64, revision Revision, windows []allocationWindow) *accounting {
	return &accounting{q: q, scheme: scheme, config: revision.Config, windows: windows,
		active: make(map[int64][]db.AllocationActiveEntriesRow), captured: map[int64]Config{revision.ID: revision.Config},
		groups: make(map[allocationWindow]sharedUsage)}
}

func (a *accounting) entries(ctx context.Context, user int64, member Share, config Config) ([]db.AllocationActiveEntriesRow, error) {
	scope := user
	if a.config.Mode == "ratio" {
		scope = 0
	}
	if rows, ok := a.active[scope]; ok {
		return rows, nil
	}
	var from, to int64
	found := false
	for _, window := range a.windows {
		if scope != 0 && config.Mode == "windows" && memberLimit(config, member, window) == 0 {
			continue
		}
		if !found {
			from, to, found = window.start, window.end, true
		} else {
			from, to = min(from, window.start), max(to, window.end)
		}
	}
	if !found {
		a.active[scope] = nil
		return nil, nil
	}
	rows, err := a.q.AllocationActiveEntries(ctx, db.AllocationActiveEntriesParams{SchemeID: a.scheme, UserID: scope, StartedFrom: from, StartedTo: to, Mode: entryMode(a.config)})
	if err != nil {
		return nil, err
	}
	if len(rows) > 4096 {
		return nil, ErrRisk
	}
	a.active[scope] = rows
	return rows, nil
}

func (a *accounting) capture(ctx context.Context, revision int64) (Config, error) {
	if config, ok := a.captured[revision]; ok {
		return config, nil
	}
	raw, err := a.q.AllocationReservationConfig(ctx, db.AllocationReservationConfigParams{ID: revision, SchemeID: a.scheme})
	if err != nil {
		return Config{}, err
	}
	var config Config
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return Config{}, err
	}
	a.captured[revision] = config
	return config, nil
}

func (a *accounting) reservations(ctx context.Context, user int64, member Share, config Config, window allocationWindow) (int64, int64, error) {
	rows, err := a.entries(ctx, user, member, config)
	if err != nil {
		return 0, 0, err
	}
	var active, held int64
	for _, row := range rows {
		if row.StartedAt < window.start || row.StartedAt >= window.end || user != 0 && row.UserID != user {
			continue
		}
		active++
		captured, err := a.capture(ctx, row.RevisionID)
		if err != nil {
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
	return active, held, nil
}

func (a *accounting) group(ctx context.Context, window allocationWindow) (sharedUsage, error) {
	if usage, ok := a.groups[window]; ok {
		return usage, nil
	}
	used, err := a.q.AllocationGroupUsage(ctx, db.AllocationGroupUsageParams{SchemeID: a.scheme, StartedFrom: window.start, StartedTo: window.end, Mode: entryMode(a.config)})
	if err != nil {
		return sharedUsage{}, err
	}
	_, held, err := a.reservations(ctx, 0, Share{}, a.config, window)
	if err != nil {
		return sharedUsage{}, err
	}
	limit := window.total(a.config)
	usage := sharedUsage{used: used, held: held, room: max(0, limit+reservation(limit)-used-held)}
	a.groups[window] = usage
	return usage, nil
}
