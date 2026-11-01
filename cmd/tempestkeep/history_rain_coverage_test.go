package main

import (
	"strings"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func TestHistoricalRainDisplayCoverage(t *testing.T) {
	if got := weekStats([]store.DayStat{{Obs: 2}}); !strings.Contains(got, "rain unavailable") {
		t.Fatalf("missing rain: %s", got)
	}
	if got := weekStats([]store.DayStat{{Obs: 2, RainReadings: 1, RainIn: new(0.0)}}); !strings.Contains(got, "rain ≥0.00 in") {
		t.Fatalf("partial dry rain: %s", got)
	}
	if got := rainCell(nil, 0, false); !strings.Contains(got, gapDot) {
		t.Fatalf("missing cell: %s", got)
	}
}

func TestHistoricalRainCellsShowPartialCoverage(t *testing.T) {
	for _, total := range []float64{0, 1.25} {
		for _, readings := range []int64{1, 2} {
			day := store.DayStat{Day: "2024-01-15", Obs: 2, RainReadings: readings, RainIn: &total}
			history := store.YearDay{Year: 2024, Obs: 2, RainReadings: readings, RainIn: &total}
			for _, output := range []string{renderWeekView([]store.DayStat{day}, 0), renderRecordsView(recordsData{thisDay: []store.YearDay{history}})} {
				if strings.Contains(output, "≥") != (readings < 2) {
					t.Fatalf("total=%v readings=%d partial coverage lost: %s", total, readings, output)
				}
			}
		}
	}
}
