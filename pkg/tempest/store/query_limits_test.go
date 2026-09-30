package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func TestQueryBoundsSQLiteValuesAndRestoresConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	writer, err := OpenWriter(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Errorf("close reader: %v", err)
		}
	})
	readLimits := func() [2]int {
		t.Helper()
		conn, err := reader.db.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Errorf("close limit connection: %v", err)
			}
		}()
		var values [2]int
		for index, id := range []int{sqlite3.SQLITE_LIMIT_LENGTH, sqlite3.SQLITE_LIMIT_COLUMN} {
			values[index], err = sqlite.Limit(conn, id, -1)
			if err != nil {
				t.Fatal(err)
			}
		}
		return values
	}
	initial := readLimits()
	for _, test := range []struct {
		name, query string
	}{
		{"blob", fmt.Sprintf("SELECT zeroblob(%d) AS synthetic_private_query", MaxQueryResultBytes+1)},
		{"string expansion", fmt.Sprintf("SELECT printf('%%*s', %d, 'x') AS synthetic_private_query", MaxQueryResultBytes+1)},
		{"columns", "SELECT " + strings.Repeat("1,", MaxQueryColumns) + "1 AS synthetic_private_query"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := reader.Query(t.Context(), test.query, 1)
			if !errors.Is(err, ErrResultTooLarge) {
				t.Fatalf("large query = %v, want ErrResultTooLarge", err)
			}
			if strings.Contains(err.Error(), "synthetic_private_query") {
				t.Fatalf("query leaked in error: %v", err)
			}
			if got := readLimits(); got != initial {
				t.Fatalf("limits after failure = %v, want %v", got, initial)
			}
			result, err := reader.Query(t.Context(), "SELECT 42", 1)
			if err != nil || result.RowCount != 1 || result.Rows[0][0] != int64(42) {
				t.Fatalf("normal query after failure = %+v, %v", result, err)
			}
		})
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	_, err = reader.Query(ctx, "WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n) SELECT sum(x) FROM n", 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("canceled query = %v, want deadline", err)
	}
	if got := readLimits(); got != initial {
		t.Fatalf("limits after cancellation = %v, want %v", got, initial)
	}
	_, err = reader.Query(t.Context(), `SELECT * FROM "too many columns in result set"`, 1)
	if !errors.Is(err, ErrArchiveIO) || errors.Is(err, ErrResultTooLarge) {
		t.Fatalf("unrelated query error misclassified as a limit: %v", err)
	}
}
