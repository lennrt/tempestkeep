package store_test

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	_ "modernc.org/sqlite"
)

// schema mirrors the collector's obs_st table (the columns the store reads).
const schema = `
CREATE TABLE obs_st (
  device_id INTEGER NOT NULL,
  epoch INTEGER NOT NULL,
  wind_lull REAL, wind_avg REAL, wind_gust REAL, wind_dir INTEGER, wind_interval INTEGER,
  pressure_mb REAL, air_temp_c REAL, humidity REAL,
  illuminance_lux REAL, uv REAL, solar_wm2 REAL,
  rain_mm REAL, precip_type INTEGER, strike_dist_km REAL, strike_count INTEGER,
  battery_v REAL, report_interval_min INTEGER, source TEXT,
  PRIMARY KEY (device_id, epoch)
);`

type fixture struct {
	epoch                     int64
	temp, gust, rain, strikes float64
	pres, solar, uv           float64
}

// fixtures: six 1-minute observations. Only one row carries rain (25.4 mm = 1 in),
// so the "wettest day" total is deterministic regardless of the test host's zone.
// Pressure, solar, and UV vary so the all-time records have a unique extreme row.
var fixtures = []fixture{
	{1700000000, 10, 3, 0, 0, 1013, 100, 1},
	{1700000060, 15, 8, 0, 1, 1013, 300, 3},
	{1700000120, 20, 12, 25.4, 0, 1013, 500, 5},
	{1700000180, 25, 20, 0, 2, 1013, 900, 8}, // hottest temp, peak gust, peak solar & UV
	{1700000240, 5, 6, 0, 0, 1000, 50, 0},    // coldest temp, lowest pressure
	{1700000300, 22, 15, 0, 3, 1013, 200, 2}, // newest row
}

func setupDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open writable db: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close temp db: %v", err)
		}
	}()
	if _, err := db.ExecContext(t.Context(), schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	const ins = `INSERT INTO obs_st
		(device_id, epoch, air_temp_c, wind_gust, wind_avg, wind_dir, humidity, pressure_mb, solar_wm2, uv, rain_mm, strike_count)
		VALUES (1, ?, ?, ?, 2.0, 180, 50.0, ?, ?, ?, ?, ?)`
	for _, f := range fixtures {
		if _, err := db.ExecContext(t.Context(), ins, f.epoch, f.temp, f.gust, f.pres, f.solar, f.uv, f.rain, f.strikes); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	return path
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), setupDB(t))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	closeOnCleanup(t, s)
	return s
}

func TestOpenMissingFile(t *testing.T) {
	if _, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "nope.sqlite")); err == nil {
		t.Error("expected error opening a database with no obs_st table")
	}
}

func TestOpenRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	target := setupDB(t)
	link := filepath.Join(t.TempDir(), "archive.sqlite")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(t.Context(), link); !errors.Is(err, store.ErrInvalidArgument) {
		t.Fatalf("Open error = %v, want ErrInvalidArgument", err)
	}
}

func TestOpenRejectsMixedDeviceArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "INSERT INTO obs_st (device_id, epoch) VALUES (1, 100), (2, 200)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(context.Background(), path); err == nil || !strings.Contains(err.Error(), "multiple device IDs") {
		t.Fatalf("Open mixed archive error = %v", err)
	}
}

func TestCoverageAndLatest(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	cov, err := s.Coverage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cov.Count != int64(len(fixtures)) {
		t.Errorf("count = %d, want %d", cov.Count, len(fixtures))
	}
	if !cov.MinEpoch.Valid || cov.MinEpoch.Int64 != 1700000000 {
		t.Errorf("min epoch = %v, want 1700000000", cov.MinEpoch)
	}
	if !cov.MaxEpoch.Valid || cov.MaxEpoch.Int64 != 1700000300 {
		t.Errorf("max epoch = %v, want 1700000300", cov.MaxEpoch)
	}

	latest, err := s.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.AirTempC == nil || *latest.AirTempC != 22 {
		t.Errorf("latest temp = %v, want 22", latest.AirTempC)
	}
	if latest.Epoch != 1700000300 {
		t.Errorf("latest epoch = %d, want 1700000300", latest.Epoch)
	}
}

func TestRecords(t *testing.T) {
	s := openStore(t)
	r, err := s.Records(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.HottestF == nil || !almost(*r.HottestF, model.CToF(25)) {
		t.Errorf("hottest = %v, want %v", r.HottestF, model.CToF(25))
	}
	if r.HottestEpoch == nil || *r.HottestEpoch != 1700000180 {
		t.Errorf("hottest epoch = %v, want 1700000180", r.HottestEpoch)
	}
	if r.ColdestF == nil || !almost(*r.ColdestF, model.CToF(5)) {
		t.Errorf("coldest = %v, want %v", r.ColdestF, model.CToF(5))
	}
	if r.PeakGustMph == nil || !almost(*r.PeakGustMph, model.MpsToMph(20)) {
		t.Errorf("peak gust = %v, want %v", r.PeakGustMph, model.MpsToMph(20))
	}
	if r.TotalStrikes == nil || *r.TotalStrikes != 6 {
		t.Errorf("total strikes = %v, want 6", r.TotalStrikes)
	}
	if r.WettestDayIn == nil || !almost(*r.WettestDayIn, 1.0) {
		t.Errorf("wettest day = %v in, want 1.0", r.WettestDayIn)
	}
	if r.LowestPressureInHg == nil || !almost(*r.LowestPressureInHg, model.MbToInHg(1000)) {
		t.Errorf("lowest pressure = %v, want %v", r.LowestPressureInHg, model.MbToInHg(1000))
	}
	if r.LowestPressureEpoch == nil || *r.LowestPressureEpoch != 1700000240 {
		t.Errorf("lowest pressure epoch = %v, want 1700000240", r.LowestPressureEpoch)
	}
	if r.PeakSolarWm2 == nil || !almost(*r.PeakSolarWm2, 900) || r.PeakSolarEpoch == nil || *r.PeakSolarEpoch != 1700000180 {
		t.Errorf("peak solar = %v at %v, want 900 at 1700000180", r.PeakSolarWm2, r.PeakSolarEpoch)
	}
	if r.PeakUV == nil || !almost(*r.PeakUV, 8) || r.PeakUVEpoch == nil || *r.PeakUVEpoch != 1700000180 {
		t.Errorf("peak UV = %v at %v, want 8 at 1700000180", r.PeakUV, r.PeakUVEpoch)
	}
}

func TestDailySummary(t *testing.T) {
	s := openStore(t)
	days, err := s.DailySummary(context.Background(), 1700000000, 1700000300)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) == 0 {
		t.Fatal("no days returned")
	}

	// Aggregate across days so assertions don't depend on the host's zone.
	var totalObs int64
	var totalRain float64
	gotMin, gotMax := math.Inf(1), math.Inf(-1)
	for _, d := range days {
		totalObs += d.Obs
		totalRain += d.RainIn
		if d.TempMinF != nil && *d.TempMinF < gotMin {
			gotMin = *d.TempMinF
		}
		if d.TempMaxF != nil && *d.TempMaxF > gotMax {
			gotMax = *d.TempMaxF
		}
	}
	if totalObs != int64(len(fixtures)) {
		t.Errorf("summed obs = %d, want %d", totalObs, len(fixtures))
	}
	if !almost(totalRain, 1.0) {
		t.Errorf("summed rain = %v in, want 1.0", totalRain)
	}
	if !almost(gotMin, model.CToF(5)) {
		t.Errorf("min temp = %v, want %v", gotMin, model.CToF(5))
	}
	if !almost(gotMax, model.CToF(25)) {
		t.Errorf("max temp = %v, want %v", gotMax, model.CToF(25))
	}
}

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-4 }
