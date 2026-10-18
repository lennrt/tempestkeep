package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

// Inspect serialized values so this regression can run unchanged against the
// previous float64 API and distinguish explicit null from an omitted total.
func TestRainStatsJSONCoverage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rain     []*float64
		readings int64
		total    *float64
	}{
		{"empty", nil, 0, nil},
		{"missing", []*float64{nil, nil}, 0, nil},
		{"measured zero", []*float64{new(0.0), new(0.0)}, 2, new(0.0)},
		{"partial zero", []*float64{new(0.0), nil}, 1, new(0.0)},
		{"positive", []*float64{new(25.4), new(12.7)}, 2, new(1.5)},
		{"partial positive", []*float64{new(25.4), nil, new(0.0)}, 2, new(1.0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const base = int64(1700000000)
			rows := make([]model.DeviceObs, len(tc.rain))
			for i, rain := range tc.rain {
				rows[i] = model.DeviceObs{Epoch: base + int64(i)*60, AirTempC: new(20.0), RainMm: rain}
			}
			s := openStoreWithDeviceObs(t, rows)
			got, err := s.RainStats(t.Context(), base, base+300)
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			raw, exists := fields["total_in"]
			if !exists {
				t.Fatal("total_in must be present even when null")
			}
			var total *float64
			if err := json.Unmarshal(raw, &total); err != nil {
				t.Fatalf("total_in must be a number or null: %s", raw)
			}
			if tc.total == nil {
				if total != nil {
					t.Errorf("missing rain encoded as %s, want null", raw)
				}
			} else if total == nil || !almost(*total, *tc.total) {
				t.Errorf("reported total=%s, want %v", raw, *tc.total)
			}
			for key, want := range map[string]int64{"observations": int64(len(rows)), "rain_readings": tc.readings} {
				raw, exists := fields[key]
				if !exists {
					t.Errorf("coverage field %s is absent", key)
					continue
				}
				var count int64
				if err := json.Unmarshal(raw, &count); err != nil || count != want {
					t.Errorf("%s=%s, want %d", key, raw, want)
				}
			}
			wantDays := int64(1)
			if len(rows) == 0 {
				wantDays = 0
			}
			if got.DaysObserved != wantDays {
				t.Errorf("days_observed=%d, want %d", got.DaysObserved, wantDays)
			}
		})
	}
}

func TestRainStatsCoverageUsesInclusiveSelectedRange(t *testing.T) {
	const base = int64(1700000000)
	s := openStoreWithDeviceObs(t, []model.DeviceObs{
		{Epoch: base, RainMm: new(25.4)},
		{Epoch: base + 60, RainMm: new(0.0)},
		{Epoch: base + 90, AirTempC: new(20.0)},
		{Epoch: base + 120, RainMm: new(2.54)},
		{Epoch: base + 180, RainMm: new(50.8)},
	})
	for _, tc := range []struct {
		name                string
		start, end          int64
		observations, reads int64
		total               *float64
	}{
		{"both endpoints included", 60, 120, 3, 2, new(0.1)},
		{"zero endpoint", 60, 60, 1, 1, new(0.0)},
		{"positive endpoint", 120, 120, 1, 1, new(0.1)},
		{"missing between rain readings", 61, 119, 1, 0, nil},
		{"empty between readings", 91, 119, 0, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.RainStats(t.Context(), base+tc.start, base+tc.end)
			if err != nil {
				t.Fatal(err)
			}
			if got.Observations != tc.observations || got.RainReadings != tc.reads {
				t.Fatalf("coverage=%d/%d, want %d/%d", got.RainReadings, got.Observations, tc.reads, tc.observations)
			}
			if tc.total == nil {
				if got.TotalIn != nil {
					t.Fatalf("rain outside the selected range produced total=%v", *got.TotalIn)
				}
			} else if got.TotalIn == nil || !almost(*got.TotalIn, *tc.total) {
				t.Fatalf("selected total=%v, want %v", got.TotalIn, *tc.total)
			}
		})
	}
}

func TestRainStatsCoverageAcrossDST(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	previous := time.Local
	time.Local = location
	t.Cleanup(func() { time.Local = previous })
	for _, tc := range []struct {
		name       string
		month      time.Month
		day, hours int
	}{
		{"spring forward", time.March, 9, 23},
		{"fall back", time.November, 2, 25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Date(2025, tc.month, tc.day, 0, 0, 0, 0, location)
			next := start.AddDate(0, 0, 1)
			if next.Sub(start) != time.Duration(tc.hours)*time.Hour {
				t.Fatal("fixture does not span the expected daylight-saving transition")
			}
			s := openStoreWithDeviceObs(t, []model.DeviceObs{
				{Epoch: start.Unix() - 1, RainMm: new(25.4)},
				{Epoch: start.Unix(), RainMm: new(0.0)},
				{Epoch: start.Add(90 * time.Minute).Unix(), AirTempC: new(20.0)},
				{Epoch: start.Add(150 * time.Minute).Unix(), RainMm: new(2.54)},
				{Epoch: next.Unix() - 1, RainMm: new(5.08)},
				{Epoch: next.Unix(), RainMm: new(50.8)},
			})
			got, err := s.RainStats(t.Context(), start.Unix(), next.Unix()-1)
			if err != nil {
				t.Fatal(err)
			}
			if got.Observations != 4 || got.RainReadings != 3 || got.DaysObserved != 1 ||
				got.TotalIn == nil || !almost(*got.TotalIn, 0.3) || got.RainyDays != 1 ||
				got.WettestDay != start.Format("2006-01-02") || got.LongestWetSpellDays != 1 {
				t.Fatalf("DST-day coverage or totals changed: %+v", got)
			}
			// Noon samples remain consecutive calendar days across a 23- or
			// 25-hour interval. Counts do not infer unrecorded samples between them.
			var dryRows []model.DeviceObs
			for offset := -1; offset <= 1; offset++ {
				date := start.AddDate(0, 0, offset)
				noon := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, location)
				dryRows = append(dryRows, model.DeviceObs{Epoch: noon.Unix(), RainMm: new(0.0)})
			}
			if span := dryRows[1].Epoch - dryRows[0].Epoch; span != int64(tc.hours)*3600 {
				t.Fatalf("noon samples span %ds, want %dh", span, tc.hours)
			}
			dry := openStoreWithDeviceObs(t, dryRows)
			spell, err := dry.RainStats(t.Context(), dryRows[0].Epoch, dryRows[2].Epoch)
			if err != nil {
				t.Fatal(err)
			}
			if spell.Observations != 3 || spell.RainReadings != 3 || spell.DaysObserved != 3 ||
				spell.TotalIn == nil || *spell.TotalIn != 0 || spell.LongestDrySpellDays != 3 ||
				spell.DrySpellStart != start.AddDate(0, 0, -1).Format("2006-01-02") ||
				spell.DrySpellEnd != start.AddDate(0, 0, 1).Format("2006-01-02") {
				t.Fatalf("DST calendar spell changed or coverage was extrapolated: %+v", spell)
			}
		})
	}
}

func TestRainStatsCoveragePreservesErrors(t *testing.T) {
	const epoch = int64(1700000000)
	s := openStoreWithDeviceObs(t, []model.DeviceObs{{Epoch: epoch, RainMm: new(0.0)}})
	for _, bounds := range [][2]int64{{-1, 0}, {epoch + 1, epoch}, {0, store.MaxRangeSeconds + 1}} {
		if _, err := s.RainStats(t.Context(), bounds[0], bounds[1]); !errors.Is(err, store.ErrInvalidArgument) {
			t.Fatalf("invalid range %v returned %v", bounds, err)
		}
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.RainStats(canceled, epoch, epoch); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation returned %v", err)
	}
	expired, stop := context.WithDeadline(t.Context(), time.Unix(1, 0))
	defer stop()
	if _, err := s.RainStats(expired, epoch, epoch); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline returned %v", err)
	}
	if got, err := s.RainStats(t.Context(), epoch, epoch); err != nil || got.TotalIn == nil || got.RainReadings != 1 {
		t.Fatalf("query after canceled reads: %+v, error=%v", got, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RainStats(t.Context(), epoch, epoch); !errors.Is(err, store.ErrClosed) {
		t.Fatalf("closed archive returned %v", err)
	}
}
