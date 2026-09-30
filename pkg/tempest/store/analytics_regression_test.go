package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func TestWindRosePreservesFractionalDirections(t *testing.T) {
	// Legacy archives may contain unnormalized directions; preserve their
	// previous wrapping behavior while retaining fractional degrees.
	for _, direction := range []float64{-348.75, -11.3, -11.25, 11.24, 11.25, 11.3, 33.74, 33.75, 348.74, 348.75, 359.9, 360, 371.25} {
		s := openStoreWith(t, []obsRow{{epoch: 1700000000, windAvg: new(2.0), windDir: new(direction)}})
		rose, err := s.WindRose(t.Context(), 1700000000, 1700000000)
		if err != nil {
			t.Fatal(err)
		}
		for _, sector := range rose.Sectors {
			if sector.Count > 0 && sector.Sector != model.Compass(direction) {
				t.Errorf("direction %v: sector = %s, want %s", direction, sector.Sector, model.Compass(direction))
			}
		}
	}
}

func TestWindRoseCountsWindWithoutDirection(t *testing.T) {
	s := openStoreWith(t, []obsRow{
		{epoch: 1700000000, windAvg: new(0.1)},
		{epoch: 1700000060, windAvg: new(2.0)},
		{epoch: 1700000120, windAvg: new(3.0), windDir: new(90.0)},
		{epoch: 1700000180, windDir: new(90.0)}, // direction alone is not a wind-speed reading
	})
	rose, err := s.WindRose(t.Context(), 1700000000, 1700000180)
	if err != nil {
		t.Fatal(err)
	}
	if rose.Obs != 3 || !almost(rose.CalmPct, 100.0/3) {
		t.Errorf("wind count/calm share = %d/%v, want 3/%v", rose.Obs, rose.CalmPct, 100.0/3)
	}
	if rose.Sectors[4].Count != 1 || rose.Sectors[4].Pct != 100 {
		t.Errorf("known east sector = %+v, want one reading and 100%% of known directions", rose.Sectors[4])
	}
}

func TestSolarActivityUsesReportedIntervals(t *testing.T) {
	s := openStoreWithDeviceObs(t, []model.DeviceObs{
		{Epoch: 1700000040, SolarWm2: new(100.0), ReportIntervalMin: new(1.0)},
		{Epoch: 1700000100, SolarWm2: new(200.0), ReportIntervalMin: new(5.0)},
	})
	got, err := s.SolarActivity(t.Context(), 1700000040, 1700000100)
	if err != nil {
		t.Fatal(err)
	}
	want := (100.0*60 + 200.0*300) / 1e6
	if !almost(got.TotalInsolationMJ, want) {
		t.Errorf("insolation = %v MJ/m², want %v from the reported intervals", got.TotalInsolationMJ, want)
	}
}

func TestSolarActivitySparseAndDenseIntervals(t *testing.T) {
	base := localNoon(2024, time.August, 1)
	for _, minutes := range []int{1, 15} {
		var rows []model.DeviceObs
		for i := range minutes {
			rows = append(rows, model.DeviceObs{
				Epoch: base + int64(i)*60, SolarWm2: new(100.0), ReportIntervalMin: new(1.0),
			})
		}
		s := openStoreWithDeviceObs(t, rows)
		got, err := s.SolarActivity(t.Context(), base, base+899)
		if err != nil {
			t.Fatal(err)
		}
		if want := float64(minutes) * 100 * 60 / 1e6; !almost(got.TotalInsolationMJ, want) {
			t.Errorf("%d one-minute samples: energy = %v, want %v", minutes, got.TotalInsolationMJ, want)
		}
	}
}

func TestSolarActivityMissingIntervalsRetainPeaks(t *testing.T) {
	base := localNoon(2024, time.August, 1)
	s := openStoreWithDeviceObs(t, []model.DeviceObs{
		{Epoch: base, SolarWm2: new(100.0), ReportIntervalMin: new(1.0)},
		{Epoch: base + 86400, SolarWm2: new(500.0), UV: new(7.0), IlluminanceLux: new(10000.0)},
		{Epoch: base + 86400 + 60, SolarWm2: new(400.0), ReportIntervalMin: new(0.0)},
		{Epoch: base + 86400 + 120, ReportIntervalMin: new(1.0)},                 // interval alone cannot establish energy
		{Epoch: base + 2*86400, SolarWm2: new(0.0), ReportIntervalMin: new(1.0)}, // a measured zero counts
	})
	got, err := s.SolarActivity(t.Context(), base, base+2*86400)
	if err != nil {
		t.Fatal(err)
	}
	if got.DaysObserved != 3 || !almost(got.TotalInsolationMJ, 0.006) ||
		got.AvgDailyInsolationMJ == nil || !almost(*got.AvgDailyInsolationMJ, 0.003) {
		t.Errorf("coverage-aware solar estimate = %+v", got)
	}
	if got.PeakSolarWm2 == nil || *got.PeakSolarWm2 != 500 || got.PeakSolarDay != "2024-08-02" ||
		got.PeakUV == nil || *got.PeakUV != 7 || got.MaxIlluminanceLux == nil || *got.MaxIlluminanceLux != 10000 {
		t.Errorf("peaks from readings without duration were lost: %+v", got)
	}
	unknown, err := s.SolarActivity(t.Context(), base+86400, base+86400+120)
	if err != nil {
		t.Fatal(err)
	}
	if unknown.AvgDailyInsolationMJ != nil || unknown.SunniestDayMJ != nil || unknown.SunniestDay != "" || unknown.TotalInsolationMJ != 0 {
		t.Errorf("unknown duration produced an energy estimate: %+v", unknown)
	}
}

func TestSolarActivityAssignsReportedIntervalByTimestamp(t *testing.T) {
	// The ten-minute report ending just after midnight belongs to Aug 2. Its
	// full interval contributes even for a one-second query; this is a sample
	// estimate, not an allocation of energy across query or calendar boundaries.
	epoch := time.Date(2024, time.August, 2, 0, 1, 0, 0, time.Local).Unix()
	s := openStoreWithDeviceObs(t, []model.DeviceObs{
		{Epoch: epoch - 60, SolarWm2: new(900.0), ReportIntervalMin: new(1.0)},
		{Epoch: epoch, SolarWm2: new(100.0), ReportIntervalMin: new(10.0)},
		{Epoch: epoch + 60, SolarWm2: new(900.0), ReportIntervalMin: new(1.0)},
	})
	got, err := s.SolarActivity(t.Context(), epoch, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if !almost(got.TotalInsolationMJ, 0.06) || got.SunniestDay != "2024-08-02" {
		t.Errorf("timestamp-selected energy = %+v, want 0.06 MJ/m² on Aug 2", got)
	}
}

func TestMissingEventReadingsBreakSpells(t *testing.T) {
	base := localNoon(2024, time.August, 1)
	s := openStoreWithDeviceObs(t, []model.DeviceObs{
		{Epoch: base, RainMm: new(0.0), StrikeCount: new(0.0)},
		{Epoch: base + 86400}, // no event reading is not a measured zero
		{Epoch: base + 2*86400, RainMm: new(0.0), StrikeCount: new(0.0)},
	})
	rain, err := s.RainStats(t.Context(), base, base+2*86400)
	if err != nil {
		t.Fatal(err)
	}
	if rain.DaysObserved != 3 || rain.TotalIn != 0 || rain.LongestDrySpellDays != 1 {
		t.Errorf("dry spell = %d, want 1 (missing rain breaks the run)", rain.LongestDrySpellDays)
	}
	lightning, err := s.LightningActivity(t.Context(), base, base+2*86400)
	if err != nil {
		t.Fatal(err)
	}
	if lightning.DaysObserved != 3 || lightning.TotalStrikes != 0 || lightning.LongestStormFreeDays != 1 {
		t.Errorf("storm-free spell = %d, want 1 (missing strikes break the run)", lightning.LongestStormFreeDays)
	}
}

func TestMissingRainBreaksWetSpell(t *testing.T) {
	base := localNoon(2024, time.August, 1)
	s := openStoreWithDeviceObs(t, []model.DeviceObs{
		{Epoch: base, RainMm: new(1.0)},
		{Epoch: base + 86400},
		{Epoch: base + 2*86400, RainMm: new(1.0)},
		{Epoch: base + 2*86400 + 60}, // missing sample within an observed day does not erase its rainfall
	})
	got, err := s.RainStats(t.Context(), base, base+2*86400+60)
	if err != nil {
		t.Fatal(err)
	}
	if got.DaysObserved != 3 || got.RainyDays != 2 || got.LongestWetSpellDays != 1 || got.LongestDrySpellDays != 0 ||
		!almost(got.TotalIn, model.MmToInch(2)) {
		t.Errorf("missing rain joined separate wet days or invented dry days: %+v", got)
	}
}

func TestComfortStatisticsRequiresRelevantSensor(t *testing.T) {
	for _, test := range []struct {
		name     string
		tempC    float64
		humidity *float64
		wind     *float64
		want     *float64
	}{
		{"hot missing humidity", 35, nil, new(2.0), nil},
		{"cold missing wind", 0, new(50.0), nil, nil},
		{"mild missing both", 20, nil, nil, new(68.0)},
		{"hot measured zero humidity", 35, new(0.0), nil, new(model.ApparentTempF(95, 0, 0))},
		{"cold measured zero wind", 0, nil, new(0.0), new(32.0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := openStoreWithDeviceObs(t, []model.DeviceObs{{
				Epoch: 1700000000, AirTempC: new(test.tempC), Humidity: test.humidity, WindAvg: test.wind,
			}})
			got, err := s.ComfortStatistics(t.Context(), 1700000000, 1700000000)
			if err != nil {
				t.Fatal(err)
			}
			if got.DaysObserved != 1 {
				t.Errorf("days observed = %d, want 1 with temperature", got.DaysObserved)
			}
			if test.want == nil {
				if got.HottestFeelsLikeF != nil || got.ColdestFeelsLikeF != nil {
					t.Errorf("missing required sensor produced apparent temperature: %+v", got)
				}
			} else if got.HottestFeelsLikeF == nil || !almost(*got.HottestFeelsLikeF, *test.want) ||
				got.ColdestFeelsLikeF == nil || !almost(*got.ColdestFeelsLikeF, *test.want) {
				t.Errorf("apparent temperatures = %+v, want %v", got, *test.want)
			}
		})
	}
}

func openStoreWithDeviceObs(t *testing.T, observations []model.DeviceObs) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "observations.sqlite")
	w, err := store.OpenWriter(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, w)
	if _, err := w.InsertObs(t.Context(), 1, observations); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, s)
	return s
}
