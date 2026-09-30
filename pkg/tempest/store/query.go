package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// QueryResult is the outcome of a read-only SQL query: column names plus rows
// of JSON-friendly values (numbers, strings, nulls).
type QueryResult struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	RowCount  int      `json:"row_count"`
	Truncated bool     `json:"truncated,omitempty"` // hit maxRows; more rows exist
}

// Query runs one read-only SELECT (or WITH … SELECT) statement and returns up
// to maxRows rows (≤0 means 1000). Two layers keep it safe: this validation
// accepts only a single SELECT/WITH statement, and the connection itself runs
// under PRAGMA query_only, so even a statement that slipped through could not
// mutate the archive. SQLite also bounds each value or encoded row before Go
// receives it; row, column, total-byte, and time limits bound the result.
func (s *Store) Query(ctx context.Context, query string, maxRows int) (_ QueryResult, err error) {
	var res QueryResult
	if len(query) == 0 || len(query) > MaxQueryBytes || strings.IndexByte(query, 0) >= 0 {
		return res, fmt.Errorf("%w: query must contain 1..%d bytes and no NUL", ErrInvalidArgument, MaxQueryBytes)
	}
	if err := validateReadOnlyQuery(query); err != nil {
		return res, err
	}
	if maxRows <= 0 {
		maxRows = MaxQueryRows
	}
	if maxRows > MaxQueryRows {
		return res, fmt.Errorf("%w: max rows exceeds %d", ErrInvalidArgument, MaxQueryRows)
	}
	db, err := s.database()
	if err != nil {
		return res, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, err := db.Conn(queryCtx)
	if err != nil {
		return res, archiveFailure("open query connection", err)
	}
	defer func() { err = errors.Join(err, archiveFailure("close query connection", conn.Close())) }()
	// Limits belong to this physical connection. Keep it checked out until
	// the result closes and restore its previous settings before reuse.
	limits := []struct{ id, value, previous int }{
		{id: sqlite3.SQLITE_LIMIT_LENGTH, value: MaxQueryResultBytes},
		{id: sqlite3.SQLITE_LIMIT_COLUMN, value: MaxQueryColumns},
	}
	for index := range limits {
		limit := &limits[index]
		limit.previous, err = sqlite.Limit(conn, limit.id, limit.value)
		if err != nil {
			return res, archiveFailure("bound query connection", err)
		}
		defer func() {
			_, restoreErr := sqlite.Limit(conn, limit.id, limit.previous)
			err = errors.Join(err, archiveFailure("restore query connection", restoreErr))
		}()
	}
	rows, err := conn.QueryContext(queryCtx, query)
	if err != nil {
		return res, queryFailure("execute query", err)
	}
	defer func() { err = errors.Join(err, archiveFailure("close query rows", rows.Close())) }()

	res.Columns, err = rows.Columns()
	if err != nil {
		return res, archiveFailure("read query columns", err)
	}
	if len(res.Columns) > MaxQueryColumns {
		return res, fmt.Errorf("%w: query returned more than %d columns", ErrResultTooLarge, MaxQueryColumns)
	}
	resultBytes := 0
	for _, column := range res.Columns {
		resultBytes += len(column)
	}
	for rows.Next() {
		if len(res.Rows) >= maxRows {
			res.Truncated = true
			break
		}
		vals := make([]any, len(res.Columns))
		ptrs := make([]any, len(vals))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return res, queryFailure("read query row", err)
		}
		for i, v := range vals {
			switch value := v.(type) {
			case nil:
			case []byte:
				resultBytes += len(value)
				vals[i] = string(value)
			case string:
				resultBytes += len(value)
			case float64:
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return res, fmt.Errorf("%w: query result contains a non-finite number", ErrInvalidArgument)
				}
				resultBytes += 8
			case int64, bool:
				resultBytes += 8
			default:
				return res, fmt.Errorf("%w: unsupported result value in column %d", ErrInvalidArchive, i)
			}
		}
		if resultBytes > MaxQueryResultBytes {
			return res, fmt.Errorf("%w: query result exceeds %d bytes", ErrResultTooLarge, MaxQueryResultBytes)
		}
		res.Rows = append(res.Rows, vals)
	}
	res.RowCount = len(res.Rows)
	return res, queryFailure("read query rows", rows.Err())
}

func queryFailure(action string, err error) error {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) && (sqliteErr.Code() == sqlite3.SQLITE_TOOBIG ||
		sqliteErr.Code() == sqlite3.SQLITE_ERROR && sqliteErr.Error() == "SQL logic error: too many columns in result set (1)") {
		// SQLite reports the column limit as its generic error code. Match
		// the complete diagnostic so an identifier with similar text cannot
		// turn an unrelated query error into a size-limit classification.
		return fmt.Errorf("%w: query exceeds SQLite value, row, or column limits", ErrResultTooLarge)
	}
	return archiveFailure(action, err)
}

// validateReadOnlyQuery accepts exactly one SELECT or WITH statement. Leading
// SQL comments are skipped; a semicolon anywhere but the very end is rejected
// (which also rejects string literals containing ';', an accepted limitation
// for a defense-in-depth check in front of a query_only connection).
func validateReadOnlyQuery(query string) error {
	q := strings.TrimSpace(query)
	for {
		switch {
		case strings.HasPrefix(q, "--"):
			if i := strings.IndexByte(q, '\n'); i >= 0 {
				q = strings.TrimSpace(q[i+1:])
				continue
			}
			q = ""
		case strings.HasPrefix(q, "/*"):
			if i := strings.Index(q, "*/"); i >= 0 {
				q = strings.TrimSpace(q[i+2:])
				continue
			}
			q = ""
		}
		break
	}
	if q == "" {
		return fmt.Errorf("%w: empty query", ErrInvalidArgument)
	}
	head := strings.ToUpper(q)
	if !strings.HasPrefix(head, "SELECT") && !strings.HasPrefix(head, "WITH") {
		return fmt.Errorf("%w: only a read-only SELECT or WITH SELECT is permitted", ErrInvalidArgument)
	}
	// SQLite lets a CTE prologue prefix DML (WITH x AS (...) DELETE ...), so a
	// WITH query must also be free of write keywords. A SELECT can't contain
	// DML, and skipping it there spares literals like SELECT 'UPDATE'. String
	// literals in a WITH can still false-positive; acceptable for a
	// defense-in-depth check in front of a query_only connection.
	if strings.HasPrefix(head, "WITH") && dmlKeyword.MatchString(head) {
		return fmt.Errorf("%w: only a read-only SELECT or WITH SELECT is permitted", ErrInvalidArgument)
	}
	if i := strings.IndexByte(strings.TrimRight(q, "; \t\r\n"), ';'); i >= 0 {
		return fmt.Errorf("%w: one SQL statement is permitted", ErrInvalidArgument)
	}
	return nil
}

var dmlKeyword = regexp.MustCompile(`\b(INSERT|UPDATE|DELETE|REPLACE|CREATE|DROP|ALTER|ATTACH|DETACH|PRAGMA|VACUUM|ANALYZE)\b`)

// cToFPtr converts a nullable Celsius reading to a °F pointer (nil when NULL).
func cToFPtr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := model.CToF(n.Float64)
	return &v
}

// mpsToMphPtr converts a nullable m/s reading to an mph pointer (nil when NULL).
func mpsToMphPtr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := model.MpsToMph(n.Float64)
	return &v
}
