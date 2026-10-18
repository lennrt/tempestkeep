package store

import (
	"database/sql"
	"strings"
	"testing"
)

func TestPressureSampleQueriesUseEpochIndex(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(t.Context(), qrySchema); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, query string
		args        []any
	}{
		{"predecessor", qryPressureAt, []any{1700000000}},
		{"eligible successor", qryPressureAfter, []any{1700000000, 1700005400}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+tc.query, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := rows.Close(); err != nil {
					t.Error(err)
				}
			}()
			indexed := false
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				indexed = indexed || strings.Contains(detail, "SEARCH obs_st USING INDEX idx_obs_epoch")
				if strings.Contains(detail, "SCAN obs_st") || strings.Contains(detail, "USE TEMP B-TREE") {
					t.Errorf("pressure candidate lookup must not scan or sort the archive: %s", detail)
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !indexed {
				t.Error("pressure candidate lookup did not search the epoch index")
			}
		})
	}
}
