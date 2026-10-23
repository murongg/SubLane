package gateway

import (
	"context"
	"strings"
	"testing"
)

func TestPersonalStatisticsUseTheUserIndex(t *testing.T) {
	s, meter := measuredGateway(t, 1)
	if _, err := s.UserStatistics(context.Background(), 1, 90); err != nil {
		t.Fatal(err)
	}
	queries := meter.take()
	checked := 0
	for _, query := range queries {
		if !strings.Contains(query.sql, "SUM(s.requests)") {
			continue
		}
		rows, err := s.db.Query("EXPLAIN QUERY PLAN "+query.sql, query.args...)
		if err != nil {
			t.Fatal(err)
		}
		indexed := false
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			indexed = indexed || strings.Contains(detail, "usage_daily_user")
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if !indexed {
			t.Errorf("personal summary scans other users: %s", strings.SplitN(query.sql, "\n", 2)[0])
		}
		checked++
	}
	if checked != 4 {
		t.Fatalf("checked %d personal summary queries, want 4", checked)
	}
}
