package main

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func runStats(t *testing.T, args ...string) (string, error) {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "report.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	}()
	previous := os.Stdout
	os.Stdout = file
	defer func() { os.Stdout = previous }()
	commandErr := cmdStats(args)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), commandErr
}

func TestE2EStatsRainCoverage(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, tc := range []struct {
		name     string
		rain     []*float64
		readings int
		total    *float64
	}{
		{"empty range", nil, 0, nil},
		{"missing", []*float64{nil, nil}, 0, nil},
		{"measured zero", []*float64{new(0.0), new(0.0)}, 2, new(0.0)},
		{"partial zero", []*float64{new(0.0), nil}, 1, new(0.0)},
		{"partial positive", []*float64{new(25.4), nil}, 1, new(1.0)},
		{"reported positive", []*float64{new(25.4), new(12.7)}, 2, new(1.5)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "archive.sqlite")
			base := time.Date(2025, time.June, 1, 12, 0, 0, 0, time.Local)
			rows := []model.DeviceObs{
				{Epoch: base.AddDate(0, 0, -1).Unix(), RainMm: new(100.0)},
				{Epoch: base.AddDate(0, 0, 1).Unix(), RainMm: new(100.0)},
			}
			for i, rain := range tc.rain {
				rows = append(rows, model.DeviceObs{Epoch: base.Unix() + int64(i*60), RainMm: rain, WindAvg: new(2.0)})
			}
			writer, err := store.OpenWriter(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			_, insertErr := writer.InsertObs(t.Context(), 456, rows)
			if err := errors.Join(insertErr, writer.Close()); err != nil {
				t.Fatal(err)
			}
			args := []string{"--db", path, "--start", "2025-06-01", "--end", "2025-06-01"}
			output, err := runStats(t, append(args, "--format", "json")...)
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Rain map[string]any `json:"rain"`
			}
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatal(err)
			}
			value, exists := result.Rain["total_in"]
			if !exists || (tc.total == nil && value != nil) {
				t.Fatalf("missing rain must encode explicit null: %s", output)
			}
			if tc.total != nil {
				got, numeric := value.(float64)
				if !numeric || math.Abs(got-*tc.total) > 1e-9 {
					t.Fatalf("measured total=%v, want%v", value, *tc.total)
				}
			}
			if result.Rain["observations"] != float64(len(tc.rain)) || result.Rain["rain_readings"] != float64(tc.readings) {
				t.Fatalf("rain coverage counts lost or included outside rows: %+v", result.Rain)
			}
			output, err = runStats(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case len(tc.rain) == 0:
				if !strings.Contains(output, "No observations in the selected date range") {
					t.Fatalf("empty range: %s", output)
				}
			case tc.readings == 0:
				if !strings.Contains(output, "No rain readings") || strings.Contains(output, "Total: 0.00") || !strings.Contains(output, "\nWind\n") {
					t.Fatalf("missing rain shown as zero or hid other weather: %s", output)
				}
			default:
				if tc.total != nil && *tc.total == 0 && !strings.Contains(output, "Total: 0.00 in") {
					t.Fatalf("measured zero lost: %s", output)
				}
				partial := tc.readings < len(tc.rain)
				if strings.Contains(output, "Partial rain coverage") != partial {
					t.Fatalf("partial warning mismatch: %s", output)
				}
				if !strings.Contains(output, "Rain readings:") {
					t.Fatalf("missing sample coverage: %s", output)
				}
			}
		})
	}
}
