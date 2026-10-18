package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func updateArchiveFixture(t *testing.T, path, query string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, updateErr := db.ExecContext(t.Context(), query, args...)
	if err := errors.Join(updateErr, db.Close()); err != nil {
		t.Fatal(err)
	}
}

func TestEachArchiveObsMetadataAndBounds(t *testing.T) {
	path := setupDB(t)
	start, end := fixtures[1].epoch, fixtures[3].epoch
	updateArchiveFixture(t, path, `UPDATE obs_st SET report_interval_min=1.25, wind_interval=3, precip_type=2, source='rest' WHERE epoch=?`, start)
	updateArchiveFixture(t, path, `UPDATE obs_st SET report_interval_min=0, wind_interval=0, precip_type=0, source='' WHERE epoch=?`, fixtures[2].epoch)
	s, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, s)
	var observations []store.ArchiveObs
	if err := s.EachArchiveObs(t.Context(), start, end, func(o store.ArchiveObs) error {
		observations = append(observations, o)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(observations) != 3 {
		t.Fatalf("got %d rows, want 3 inclusive rows", len(observations))
	}
	for i, got := range observations {
		want := fixtures[i+1]
		if got.Epoch != want.epoch || got.DeviceID == nil || *got.DeviceID != 1 || got.AirTempC == nil || *got.AirTempC != want.temp {
			t.Fatalf("lost sensor/order/identity in row %d: %+v", i, got)
		}
	}
	first, zero, missing := observations[0], observations[1], observations[2]
	if first.ReportIntervalMin == nil || *first.ReportIntervalMin != 1.25 || first.WindIntervalSeconds == nil || *first.WindIntervalSeconds != 3 || first.PrecipType == nil || *first.PrecipType != 2 || first.Source == nil || *first.Source != "rest" {
		t.Fatalf("stored metadata lost: %+v", first)
	}
	if zero.ReportIntervalMin == nil || *zero.ReportIntervalMin != 0 || zero.WindIntervalSeconds == nil || *zero.WindIntervalSeconds != 0 || zero.PrecipType == nil || *zero.PrecipType != 0 || zero.Source == nil || *zero.Source != "" {
		t.Fatalf("stored zero/empty values became missing: %+v", zero)
	}
	if missing.ReportIntervalMin != nil || missing.WindIntervalSeconds != nil || missing.PrecipType != nil || missing.Source != nil {
		t.Fatalf("missing metadata was invented: %+v", missing)
	}
}

func TestEachArchiveObsLegacySchema(t *testing.T) {
	for _, optionalColumns := range []bool{false, true} {
		name := "missing columns"
		if optionalColumns {
			name = "uppercase optional columns"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "legacy.sqlite")
			extra := ""
			if optionalColumns {
				extra = ", REPORT_INTERVAL_MIN REAL, SOURCE TEXT"
			}
			updateArchiveFixture(t, path, `CREATE TABLE obs_st (
				epoch INTEGER, wind_lull REAL, wind_avg REAL, wind_gust REAL, wind_dir REAL,
				pressure_mb REAL, air_temp_c REAL, humidity REAL, illuminance_lux REAL,
				uv REAL, solar_wm2 REAL, rain_mm REAL, strike_dist_km REAL,
				strike_count REAL, battery_v REAL`+extra+`)`)
			updateArchiveFixture(t, path, "INSERT INTO obs_st (epoch, air_temp_c) VALUES (1700000000, 20)")
			if optionalColumns {
				updateArchiveFixture(t, path, "UPDATE obs_st SET REPORT_INTERVAL_MIN=2, SOURCE='legacy'")
			}
			s, err := store.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			closeOnCleanup(t, s)
			calls := 0
			if err := s.EachArchiveObs(t.Context(), 0, 1800000000, func(o store.ArchiveObs) error {
				calls++
				if o.AirTempC == nil || *o.AirTempC != 20 || o.DeviceID != nil || o.WindIntervalSeconds != nil || o.PrecipType != nil {
					t.Fatalf("legacy sensor or missing metadata changed: %+v", o)
				}
				if optionalColumns {
					if o.ReportIntervalMin == nil || *o.ReportIntervalMin != 2 || o.Source == nil || *o.Source != "legacy" {
						t.Fatalf("case-insensitive optional columns lost: %+v", o)
					}
				} else if o.ReportIntervalMin != nil || o.Source != nil {
					t.Fatalf("legacy metadata was invented: %+v", o)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("got %d callbacks, want 1", calls)
			}
		})
	}
}

func TestEachArchiveObsSourceBounds(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		invalid      bool
	}{
		{"control bytes", "synthetic\x00\x01\t\n\"source\"", false},
		{"exact ASCII bound", strings.Repeat("a", store.MaxArchiveSourceBytes), false},
		{"exact Unicode bound", strings.Repeat("é", store.MaxArchiveSourceBytes/2), false},
		{"oversized ASCII", strings.Repeat("a", store.MaxArchiveSourceBytes+1), true},
		{"oversized Unicode", strings.Repeat("é", store.MaxArchiveSourceBytes/2+1), true},
		{"oversized after NUL", "\x00" + strings.Repeat("a", store.MaxArchiveSourceBytes), true},
		{"invalid UTF8", string([]byte{0xff}), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := setupDB(t)
			updateArchiveFixture(t, path, "UPDATE obs_st SET source=CAST(? AS TEXT)", tc.source)
			s, err := store.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			closeOnCleanup(t, s)
			calls := 0
			err = s.EachArchiveObs(t.Context(), fixtures[0].epoch, fixtures[0].epoch, func(o store.ArchiveObs) error {
				calls++
				if o.Source == nil || *o.Source != tc.source {
					t.Fatalf("source was truncated or changed: %+v", o.Source)
				}
				return nil
			})
			if tc.invalid {
				if !errors.Is(err, store.ErrInvalidArchive) || calls != 0 {
					t.Fatalf("invalid source: error=%v callbacks=%d", err, calls)
				}
			} else if err != nil || calls != 1 {
				t.Fatalf("valid source: error=%v callbacks=%d", err, calls)
			}
			// Even an early source error must release the store's one connection.
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if _, err := s.Coverage(ctx); err != nil {
				t.Fatalf("iterator did not release its rows: %v", err)
			}
		})
	}
}

func TestEachArchiveObsStopsAndClosesRows(t *testing.T) {
	s := openStore(t)
	sentinel := errors.New("callback stopped")
	calls := 0
	err := s.EachArchiveObs(t.Context(), 0, 1800000000, func(store.ArchiveObs) error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("callback failure: calls=%d, error=%v", calls, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls = 0
	err = s.EachArchiveObs(ctx, 0, 1800000000, func(store.ArchiveObs) error {
		calls++
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("cancellation: calls=%d, error=%v", calls, err)
	}
	checkCtx, stop := context.WithTimeout(t.Context(), time.Second)
	defer stop()
	if _, err := s.Coverage(checkCtx); err != nil {
		t.Fatalf("early exit did not release its rows: %v", err)
	}
}

func TestEachArchiveObsRejectsInvalidOperations(t *testing.T) {
	s := openStore(t)
	callback := func(store.ArchiveObs) error { t.Error("unexpected callback"); return nil }
	for _, limits := range [][2]int64{{-1, 1}, {2, 1}, {0, store.MaxRangeSeconds + 1}} {
		if err := s.EachArchiveObs(t.Context(), limits[0], limits[1], callback); !errors.Is(err, store.ErrInvalidArgument) {
			t.Fatalf("range %v: error=%v", limits, err)
		}
	}
	if err := s.EachArchiveObs(t.Context(), 0, 1, nil); !errors.Is(err, store.ErrInvalidArgument) {
		t.Fatalf("nil callback: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.EachArchiveObs(ctx, 0, 1800000000, callback); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled before start: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.EachArchiveObs(t.Context(), 0, 1, callback); !errors.Is(err, store.ErrClosed) {
		t.Fatalf("closed store: %v", err)
	}
}

func TestEachArchiveObsRejectsNonFiniteMetadata(t *testing.T) {
	path := setupDB(t)
	updateArchiveFixture(t, path, "UPDATE obs_st SET report_interval_min=1e999")
	s, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, s)
	err = s.EachArchiveObs(t.Context(), 0, 1800000000, func(store.ArchiveObs) error {
		t.Fatal("non-finite metadata reached the callback")
		return nil
	})
	if !errors.Is(err, store.ErrInvalidArchive) {
		t.Fatalf("non-finite metadata: %v", err)
	}
}
