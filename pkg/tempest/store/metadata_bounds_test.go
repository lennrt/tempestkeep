package store_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

func TestMetadataReadBoundsAcrossDatabaseEncodings(t *testing.T) {
	for _, encoding := range []string{"UTF-8", "UTF-16le", "UTF-16be"} {
		t.Run(encoding, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "archive.sqlite")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			closeOnCleanup(t, db)
			// Create a nullable, untyped legacy metadata table so external SQL
			// can also exercise NULL and non-TEXT storage classes.
			if _, err := db.ExecContext(t.Context(), fmt.Sprintf("PRAGMA encoding='%s'; CREATE TABLE meta (key TEXT PRIMARY KEY, value)", encoding)); err != nil {
				t.Fatal(err)
			}
			w, err := store.OpenWriter(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			closeOnCleanup(t, w)
			s, err := store.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			closeOnCleanup(t, s)
			for _, tc := range []struct {
				name    string
				value   any
				want    string
				invalid bool
			}{
				{"ASCII boundary", strings.Repeat("x", store.MaxMetadataValueBytes), strings.Repeat("x", store.MaxMetadataValueBytes), false},
				{"CJK boundary", strings.Repeat("中", store.MaxMetadataValueBytes/3) + "x", strings.Repeat("中", store.MaxMetadataValueBytes/3) + "x", false},
				{"supplementary boundary", strings.Repeat("🌧", store.MaxMetadataValueBytes/4), strings.Repeat("🌧", store.MaxMetadataValueBytes/4), false},
				{"ASCII above boundary", strings.Repeat("x", store.MaxMetadataValueBytes+1), "", true},
				{"CJK above boundary", strings.Repeat("中", store.MaxMetadataValueBytes/3+1), "", true},
				{"CJK encoded boundary", strings.Repeat("中", store.MaxMetadataValueBytes/2), "", true},
				{"hostile oversized", strings.Repeat("x", 1<<20), "", true},
				{"embedded NUL", "0\x00", "", true},
				{"NULL", nil, "", true},
				{"integer", int64(123), "123", false},
				{"real", 1.5, "1.5", false},
				{"blob boundary", []byte(strings.Repeat("é", store.MaxMetadataValueBytes/2)), strings.Repeat("é", store.MaxMetadataValueBytes/2), false},
				{"blob above boundary", []byte(strings.Repeat("x", store.MaxMetadataValueBytes+1)), "", true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if value, ok := tc.value.(string); ok && !tc.invalid {
						if err := w.SetMeta(t.Context(), "test", value); err != nil {
							t.Fatalf("writer rejected valid UTF-8 value: %v", err)
						}
					} else if _, err := db.ExecContext(t.Context(), "INSERT OR REPLACE INTO meta (key, value) VALUES ('test', ?)", tc.value); err != nil {
						t.Fatal(err)
					}
					for _, reader := range []interface {
						Meta(context.Context, string) (string, bool, error)
					}{s, w} {
						got, found, err := reader.Meta(t.Context(), "test")
						if tc.invalid {
							if !errors.Is(err, store.ErrInvalidArchive) || got != "" || found {
								t.Fatalf("invalid metadata: bytes=%d found=%v err=%v", len(got), found, err)
							}
						} else if err != nil || !found || got != tc.want {
							t.Fatalf("valid metadata: bytes=%d want=%d found=%v err=%v", len(got), len(tc.want), found, err)
						}
					}
				})
			}
		})
	}
}
