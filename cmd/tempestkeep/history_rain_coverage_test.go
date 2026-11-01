package main

import (
	"strings"
	"testing"
	"time"

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

func TestDayRainSparklinePreservesMissingAndReportedReadings(t *testing.T) {
	start := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		name     string
		readings int64
		total    float64
		want     string
	}{
		{"missing", 0, 0, gapDot},
		{"measured zero", 2, 0, string(blocks8[0])},
		{"partial zero", 1, 0, string(blocks8[0])},
		{"partial positive", 1, 1.25, string(blocks8[len(blocks8)-1])},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := renderDayView(dayData{
				points: []store.SeriesPoint{{
					Epoch: start.Unix(), Obs: 2, RainReadings: tc.readings, RainIn: tc.total,
				}},
				stat: &store.DayStat{
					Day: start.Format(time.DateOnly), Obs: 2, RainReadings: tc.readings,
					RainIn: historicalRainTotal(tc.total, tc.readings),
				},
			})
			if tc.readings == 0 && !strings.Contains(output, "rain unavailable") || tc.readings == 1 && !strings.Contains(output, "rain ≥") {
				t.Fatalf("missing day coverage: %s", output)
			}
			for line := range strings.SplitSeq(output, "\n") {
				chart, found := strings.CutPrefix(line, faint().Render("rain  "))
				if !found {
					continue
				}
				want := tc.want
				if tc.readings == 0 {
					want = faint().Render(want)
				}
				if tc.readings == 0 && !strings.HasPrefix(chart, want) || tc.readings > 0 && !strings.Contains(chart, want) {
					t.Fatalf("rain sparkline = %q, want first reading %q", chart, tc.want)
				}
				return
			}
			t.Fatalf("missing rain sparkline: %s", output)
		})
	}
}
