package store

import (
	"database/sql"
	"testing"
)

// TestEmbeddedQueriesCompile proves every embedded .sql file is (a) loaded by
// queries.go and (b) valid SQL against the real schema: it creates a fresh
// in-memory database from schema.sql, then Prepares every other statement
// against it. Templates are rendered first (latest.sql with the shared column
// list, epoch_of.sql with a real column name).
func TestEmbeddedQueriesCompile(t *testing.T) {
	entries, err := sqlFS.ReadDir("sql")
	if err != nil {
		t.Fatalf("read embedded sql dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no embedded .sql files")
	}

	// Every file in sql/ must have been loaded at package init; a stray file
	// nothing references is a mistake worth failing on.
	if got, want := len(entries), len(loadedSQL); got != want {
		for _, e := range entries {
			if !loadedSQL[e.Name()] {
				t.Errorf("sql/%s is embedded but never loaded by queries.go", e.Name())
			}
		}
		t.Fatalf("sql/ holds %d files but queries.go loaded %d", got, want)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close in-memory database: %v", err)
		}
	})
	// One connection: each pooled connection would otherwise get its own empty
	// in-memory database without the schema.
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(t.Context(), qrySchema); err != nil {
		t.Fatalf("schema.sql: %v", err)
	}

	for _, e := range entries {
		name := e.Name()
		var q string
		switch name {
		case "schema.sql":
			continue // executed above; it is the schema itself
		case "latest.sql":
			q = qryLatest // rendered with obsColumns at init
		case "range.sql":
			q = qryRange // rendered with obsColumns at init
		case "epoch_of.sql":
			q = epochOfSQL("air_temp_c") // any real obs_st column
		default:
			q = mustQuery(name)
		}
		func() {
			stmt, err := db.PrepareContext(t.Context(), q)
			if err != nil {
				t.Errorf("%s does not compile against the schema: %v", name, err)
				return
			}
			defer func() {
				if err := stmt.Close(); err != nil {
					t.Errorf("close %s statement: %v", name, err)
				}
			}()
		}()
	}
}
