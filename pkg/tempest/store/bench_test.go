package store_test

// Benchmarks cover analytics queries over one year of 1-minute observations.
// Run them with `make bench`. Record the source revision, Go version, operating
// system, architecture, command, and result before making a performance claim.

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	_ "modernc.org/sqlite"
)

const benchYearRows = 365 * 24 * 60 // one year at 1-minute cadence

var (
	benchOnce sync.Once
	benchPath string
	benchErr  error
)

// benchArchive lazily builds the shared one-year archive in a temp dir that
// lives for the whole test binary.
func benchArchive(b *testing.B) *store.Store {
	b.Helper()
	benchOnce.Do(func() {
		dir, err := os.MkdirTemp("", "tempestkeep-bench")
		if err != nil {
			benchErr = err
			return
		}
		benchPath = filepath.Join(dir, "bench.sqlite")
		benchErr = buildBenchArchive(b.Context(), benchPath)
	})
	if benchErr != nil {
		b.Fatalf("build bench archive: %v", benchErr)
	}
	s, err := store.Open(b.Context(), benchPath)
	if err != nil {
		b.Fatalf("store.Open: %v", err)
	}
	b.Cleanup(func() {
		if err := s.Close(); err != nil {
			b.Errorf("close benchmark store: %v", err)
		}
	})
	return s
}

// buildBenchArchive writes a year of synthetic but weather-shaped data: a
// seasonal + diurnal temperature curve, rotating wind, and periodic rain, so
// the aggregates exercise realistic value distributions rather than constants.
func buildBenchArchive(ctx context.Context, path string) (err error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	if _, err := db.ExecContext(ctx, schema+`CREATE INDEX idx_obs_epoch ON obs_st(epoch);`); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO obs_st
		(device_id, epoch, air_temp_c, humidity, pressure_mb, wind_avg, wind_gust, wind_dir, rain_mm, strike_count, uv, solar_wm2)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, stmt.Close()) }()

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	for i := range benchYearRows {
		epoch := start + int64(i)*60
		dayFrac := float64(i%1440) / 1440
		yearFrac := float64(i) / benchYearRows
		temp := 15 + 10*math.Sin(2*math.Pi*yearFrac) + 5*math.Sin(2*math.Pi*dayFrac)
		wind := 2 + 3*math.Abs(math.Sin(2*math.Pi*dayFrac))
		var rain float64
		if i%997 == 0 { // occasional rain minute
			rain = 0.5
		}
		if _, err := stmt.ExecContext(ctx, epoch, temp, 50.0, 1013.0, wind, wind*1.8,
			float64((i*7)%360), rain, i%9973/9972, temp/10, 400*dayFrac); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func BenchmarkRecords(b *testing.B) {
	s := benchArchive(b)
	ctx := b.Context()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Records(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDailySummary30Days(b *testing.B) {
	s := benchArchive(b)
	ctx := context.Background()
	start := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC).Unix()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.DailySummary(ctx, start, start+30*86400); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkThisDay(b *testing.B) {
	s := benchArchive(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.ThisDay(ctx, 6, 15); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPeriodSummaryYearByMonth(b *testing.B) {
	s := benchArchive(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.PeriodSummary(ctx, store.PeriodMonth, 0, math.MaxInt32); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWindRoseAllTime(b *testing.B) {
	s := benchArchive(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.WindRose(ctx, 0, math.MaxInt32); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLightningActivityAllTime(b *testing.B) {
	s := benchArchive(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.LightningActivity(ctx, 0, math.MaxInt32); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSolarActivityAllTime(b *testing.B) {
	s := benchArchive(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.SolarActivity(ctx, 0, math.MaxInt32); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWindStatisticsAllTime(b *testing.B) {
	s := benchArchive(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.WindStatistics(ctx, 0, math.MaxInt32); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkComfortStatisticsAllTime(b *testing.B) {
	s := benchArchive(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.ComfortStatistics(ctx, 0, math.MaxInt32); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSensorHealthAllTime(b *testing.B) {
	s := benchArchive(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.SensorHealthReport(ctx, 0, math.MaxInt32); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPressureStatisticsAllTime(b *testing.B) {
	s := benchArchive(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.PressureStatistics(ctx, 0, math.MaxInt32); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTemperatureSpellsAllTime(b *testing.B) {
	s := benchArchive(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.TemperatureSpells(ctx, store.TempSpellParams{}, 0, math.MaxInt32); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTemperatureTrend(b *testing.B) {
	s := benchArchive(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.TemperatureTrend(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSeriesWeekHourly(b *testing.B) {
	s := benchArchive(b)
	ctx := context.Background()
	start := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC).Unix()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Series(ctx, start, start+7*86400, 3600); err != nil {
			b.Fatal(err)
		}
	}
}
