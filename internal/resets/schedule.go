package resets

import "time"

func nextOccurrence(c Config, after time.Time, loc *time.Location) time.Time {
	if !c.Scheduled {
		return time.Time{}
	}
	clock, err := time.Parse("15:04", c.Time)
	if err != nil {
		return time.Time{}
	}
	date := after.In(loc)
	// Add civil dates from noon so an unrelated midnight gap cannot shift the selected day.
	date = time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, loc)
	if c.Frequency == "once" {
		parsed, err := time.Parse("2006-01-02", c.Date)
		if err != nil {
			return time.Time{}
		}
		date = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 12, 0, 0, 0, loc)
	}
	// Skipping a weekly DST gap can put the next valid occurrence in the following week.
	for offset := 0; offset < 15; offset++ {
		day := date.AddDate(0, 0, offset)
		candidate := time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
		// Civil times in a DST gap do not exist; a repeated hour still gets only one occurrence.
		if candidate.Format("2006-01-02 15:04") != day.Format("2006-01-02")+" "+c.Time {
			if c.Frequency == "once" {
				return time.Time{}
			}
			continue
		}
		if candidate.After(after) && (c.Frequency != "weekly" || int(day.Weekday()) == c.Weekday) {
			return candidate
		}
		if c.Frequency == "once" {
			break
		}
	}
	return time.Time{}
}

func timestamp(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func dayBounds(now time.Time, loc *time.Location) (int64, int64) {
	local := now.In(loc)
	noon := time.Date(local.Year(), local.Month(), local.Day(), 12, 0, 0, 0, loc)
	return civilStart(noon, loc), civilStart(noon.AddDate(0, 0, 1), loc)
}

func civilStart(noon time.Time, loc *time.Location) int64 {
	// Midnight can be absent or repeated; find the first instant of the civil date instead of normalizing 00:00.
	date := noon.Format("2006-01-02")
	low, high := noon.Add(-36*time.Hour).Unix(), noon.Unix()
	for low < high {
		middle := low + (high-low)/2
		if time.Unix(middle, 0).In(loc).Format("2006-01-02") < date {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low
}
