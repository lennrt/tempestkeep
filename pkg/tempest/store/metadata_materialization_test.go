package store

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"modernc.org/sqlite"
)

var metadataTestDriverID atomic.Uint64

func TestMetadataViewEvaluatesValueOnce(t *testing.T) {
	var calls int
	d := &sqlite.Driver{}
	if err := d.RegisterScalarFunction("metadata_value", 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
		calls++
		if calls%2 == 0 {
			return strings.Repeat("x", 1<<20), nil
		}
		return "1", nil
	}); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("metadata_test_%d", metadataTestDriverID.Add(1))
	sql.Register(name, d)
	db, err := sql.Open(name, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	// A view backed by a table can be flattened by SQLite, reevaluating its
	// expression separately for the size guard, value, and reported size.
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE payload (key TEXT);
		INSERT INTO payload VALUES ('test');
		CREATE VIEW meta AS SELECT key, metadata_value() AS value FROM payload`); err != nil {
		t.Fatal(err)
	}
	value, found, err := readMetadata(t.Context(), db, "test")
	if err != nil || !found || value != "1" || calls != 1 {
		t.Fatalf("value=%q found=%v err=%v calls=%d; want one evaluation of the valid value", value, found, err, calls)
	}
	value, found, err = readMetadata(t.Context(), db, "test")
	if !errors.Is(err, ErrInvalidArchive) || found || value != "" || calls != 2 {
		t.Fatalf("value bytes=%d found=%v err=%v calls=%d; want one evaluation of the oversized value", len(value), found, err, calls)
	}
}
