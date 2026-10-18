package store_test

import (
	"encoding/json"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

func TestSeriesEventSensorCoverage(t *testing.T) {
	const base = int64(1700000100) // aligned to five-minute buckets
	s := openStoreWithDeviceObs(t, []model.DeviceObs{
		{Epoch: base, AirTempC: new(12.0)},
		{Epoch: base + 300, RainMm: new(0.0), StrikeCount: new(0.0)},
		{Epoch: base + 600, RainMm: new(2.54), StrikeCount: new(2.0)},
		{Epoch: base + 660, RainMm: new(0.0)},
		{Epoch: base + 720, StrikeCount: new(3.0)},
		{Epoch: base + 780},
		{Epoch: base + 900, RainMm: new(0.0)},
		// Outside the requested range, even though it falls in the last bucket.
		{Epoch: base + 960, RainMm: new(25.4), StrikeCount: new(99.0)},
	})
	points, err := s.Series(t.Context(), base, base+900, 300)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 4 {
		t.Fatalf("got %d buckets, want 4", len(points))
	}
	// Assert the wire contract too: zero counts must be present, not omitted.
	encoded, err := json.Marshal(points)
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]float64
	// Time is a string, so decode only the fields under test as raw messages.
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	keys := []string{"obs", "rain_readings", "lightning_readings", "rain_in", "lightning_strikes"}
	for i, row := range raw {
		values := make(map[string]float64)
		for _, key := range keys {
			value, ok := row[key]
			if !ok {
				t.Fatalf("bucket %d is missing %s: %s", i, key, encoded)
			}
			var number float64
			if err := json.Unmarshal(value, &number); err != nil {
				t.Fatal(err)
			}
			values[key] = number
		}
		got = append(got, values)
	}
	wants := [][]float64{
		{1, 0, 0, 0, 0},  // unavailable events
		{1, 1, 1, 0, 0},  // measured zero events
		{4, 2, 2, .1, 5}, // independent, partial coverage and sums
		{1, 1, 0, 0, 0},  // rain available, lightning unavailable
	}
	for i, want := range wants {
		for j, key := range keys {
			if !almost(got[i][key], want[j]) {
				t.Errorf("bucket %d %s = %v, want %v", i, key, got[i][key], want[j])
			}
		}
	}
}
