package store

import (
	"database/sql"
	"strings"
	"testing"
)

func TestArchiveSourceBoundIsAppliedInsideSQLite(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := db.ExecContext(t.Context(), qrySchema); err != nil {
		t.Fatal(err)
	}
	const epoch = 1700000000
	if _, err := db.ExecContext(t.Context(), "INSERT INTO obs_st(device_id, epoch, source) VALUES (1, ?, ?)", epoch, strings.Repeat("a", MaxArchiveSourceBytes+1)); err != nil {
		t.Fatal(err)
	}
	projection, err := archiveColumns(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(t.Context(), execTemplate(archiveRangeTmpl, projection), epoch, epoch)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	if !rows.Next() {
		t.Fatalf("missing synthetic row: %v", rows.Err())
	}
	var device, size sql.NullInt64
	var interval, wind, precip sql.NullFloat64
	var source sql.NullString
	if _, err := scanObs(rows, &device, &interval, &wind, &precip, &source, &size); err != nil {
		t.Fatal(err)
	}
	if source.Valid || !size.Valid || size.Int64 != MaxArchiveSourceBytes+1 {
		t.Fatalf("query loaded the oversized source instead of its byte count: valid=%v, size=%+v", source.Valid, size)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
