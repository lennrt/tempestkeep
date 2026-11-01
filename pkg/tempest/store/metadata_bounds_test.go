package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func TestMetadataReadBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	w, err := store.OpenWriter(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, w)
	if err := w.SetMeta(t.Context(), "test", "0"); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, s)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, db)
	for _, tc := range []struct {
		name, value string
		invalid     bool
	}{
		{"empty", "", false},
		{"at byte limit", strings.Repeat("é", store.MaxMetadataValueBytes/2), false},
		{"above byte limit", strings.Repeat("é", store.MaxMetadataValueBytes/2) + "x", true},
		{"hostile oversized", strings.Repeat("x", 16<<20), true},
		{"embedded NUL", "0\x00", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Bypass the writer guard to model an externally modified archive.
			if _, err := db.ExecContext(t.Context(), "UPDATE meta SET value = ? WHERE key = 'test'", tc.value); err != nil {
				t.Fatal(err)
			}
			for _, reader := range []interface {
				Meta(context.Context, string) (string, bool, error)
			}{s, w} {
				value, found, err := reader.Meta(t.Context(), "test")
				if tc.invalid {
					if !errors.Is(err, store.ErrInvalidArchive) || value != "" || found {
						t.Fatalf("invalid metadata: bytes=%d found=%v err=%v", len(value), found, err)
					}
				} else if err != nil || !found || value != tc.value {
					t.Fatalf("valid metadata: bytes=%d found=%v err=%v", len(value), found, err)
				}
			}
		})
	}
}
