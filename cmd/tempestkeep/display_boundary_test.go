package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/config"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func TestListDevicesTextSanitizesAPILabels(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"stations": []api.Station{{
			StationID: 123, Name: "Synthetic station", Timezone: "UTC\x1b[2J\nforged line",
			Devices: []api.Device{{DeviceID: 456, DeviceType: "ST\x1b[31m", SerialNumber: "synthetic\x1b]0;changed-title\a"}},
		}}})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TEMPEST_TOKEN", "synthetic-token")
	t.Setenv("TEMPEST_API_BASE", srv.URL)
	t.Setenv("TEMPEST_CACHE_TTL", "0")
	file, err := os.Create(filepath.Join(t.TempDir(), "stdout.txt"))
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, file)
	previous := os.Stdout
	os.Stdout = file
	t.Cleanup(func() { os.Stdout = previous })
	if err := cmdListDevices(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	text, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.ContainsAny(text, "\x1b\a\r") || bytes.Contains(text, []byte("\nforged")) {
		t.Fatalf("API labels reached terminal without normalization: %q", text)
	}
}

func TestForecastFirstEntryIsNotAlwaysToday(t *testing.T) {
	now := time.Date(2024, 7, 15, 12, 0, 0, 0, time.Local)
	for _, offset := range []int{-1, 1} {
		day := now.AddDate(0, 0, offset)
		if got := weekdayLabel(day.Unix(), now); got == "Today" {
			t.Fatalf("forecast date %s was labeled Today", day.Format(time.DateOnly))
		}
	}
	if got := weekdayLabel(now.Unix(), now); got != "Today" {
		t.Fatalf("current forecast date label=%q, want Today", got)
	}
}

func TestListDevicesTextReturnsOutputFailure(t *testing.T) {
	for _, stations := range [][]api.Station{nil, {{StationID: 123, Name: "Synthetic"}}} {
		if err := writeDevicesText(failedOutput{}, stations); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("output error=%v, want closed pipe", err)
		}
	}
}

func TestPowerShellCommandArgPreservesLiteralPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell-specific command example")
	}
	path := "C:\\weather $archive\\owner's `station.sqlite"
	if got, want := commandArg(path), "'C:\\weather $archive\\owner''s `station.sqlite'"; got != want {
		t.Fatalf("PowerShell path quoting=%q, want %q", got, want)
	}
}

func TestDayChartRetainsQuarterHourTimezoneBuckets(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("synthetic-quarter-hour", 5*3600+45*60)
	t.Cleanup(func() { time.Local = previous })
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
	first := start.Unix() / dayBucketSeconds * dayBucketSeconds
	last := (start.AddDate(0, 0, 1).Unix() - 1) / dayBucketSeconds * dayBucketSeconds
	var points []store.SeriesPoint
	for epoch := first; epoch <= last; epoch += dayBucketSeconds {
		points = append(points, store.SeriesPoint{Epoch: epoch, Obs: 1, TempAvgF: new(50.0)})
	}
	text := renderDayView(dayData{points: points, stat: &store.DayStat{Day: "2024-01-01", Obs: 49}})
	rows := strings.Split(text, "\n")
	for _, row := range rows[:4] {
		if strings.Count(row, "█") == 49 {
			return
		}
	}
	t.Fatalf("day chart omitted populated edge buckets:\n%s", text)
}

func TestDaysInCountsCalendarDaysAcrossSpringForward(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	previous := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = previous })
	start := time.Date(2024, 3, 1, 0, 0, 0, 0, loc)
	if got := daysIn(start, start.AddDate(0, 1, 0)); got != 31 {
		t.Fatalf("March observed-day denominator=%d, want 31", got)
	}
}

func TestWeekViewKeepsRainWhenTemperatureIsMissing(t *testing.T) {
	day := time.Now().Format(time.DateOnly)
	text := renderWeekView([]store.DayStat{{Day: day, Obs: 12, RainIn: 1.25, PeakGustMph: new(18.0)}}, 0)
	if strings.Contains(text, "no observations") || !strings.Contains(text, "1.25") || !strings.Contains(text, "18") {
		t.Fatalf("missing temperature hid other observations:\n%s", text)
	}
	start, _, _ := periodRange(viewWeek, 0, time.Now())
	text = renderWeekView([]store.DayStat{
		{Day: start.Format(time.DateOnly), Obs: 12, RainIn: 1.25},
		{Day: start.AddDate(0, 0, 1).Format(time.DateOnly), Obs: 12, TempMinF: new(55.0), TempMaxF: new(70.0)},
	}, 0)
	rows := strings.Split(text, "\n")
	missing, present := strings.Split(rows[0], " rain ")[0], strings.Split(rows[1], " rain ")[0]
	if lipgloss.Width(missing) != lipgloss.Width(present) {
		t.Fatalf("rain columns differ with missing temperature:\n%s", text)
	}
}

func TestStatsEmptyAnalysisRangeDoesNotClaimNoLightning(t *testing.T) {
	report := statsReport{cov: store.Coverage{
		Count: 1, MinEpoch: sql.NullInt64{Int64: 1700000000, Valid: true}, MaxEpoch: sql.NullInt64{Int64: 1700000000, Valid: true},
	}}
	var output strings.Builder
	if err := writeStats(&output, report); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "none detected") || strings.Contains(output.String(), "calm 0%") || !strings.Contains(output.String(), "No observations in the selected date range") {
		t.Fatalf("missing range observations reported as measured zeros:\n%s", output.String())
	}
}

func TestStatsMissingWindIsNotMeasuredCalm(t *testing.T) {
	report := statsReport{
		cov:  store.Coverage{Count: 1, MinEpoch: sql.NullInt64{Int64: 1700000000, Valid: true}, MaxEpoch: sql.NullInt64{Int64: 1700000000, Valid: true}},
		rain: store.RainStats{DaysObserved: 1},
	}
	var output strings.Builder
	if err := writeStats(&output, report); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "calm 0%") || !strings.Contains(output.String(), "calm —") {
		t.Fatalf("missing wind was reported as measured zero:\n%s", output.String())
	}
}

func TestWriteEnvFileRejectsOversizedOutputBeforeReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	var originalText strings.Builder
	for i := range 128 {
		fmt.Fprintf(&originalText, "EXISTING_%04d=%s\n", i, strings.Repeat("a", 8176))
	}
	original := []byte(originalText.String())
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	existing, err := readEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err = writeEnvFile(path, existing, map[string]string{"TEMPEST_TOKEN": strings.Repeat("a", 2000)})
	if !errors.Is(err, config.ErrInvalidConfig) {
		t.Fatalf("oversized output error=%v, want invalid config", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("oversized output changed the existing file: len=%d err=%v", len(got), err)
	}
}

func TestStatsLightningZeroRequiresCoverageQualification(t *testing.T) {
	for _, count := range []*float64{nil, new(0.0)} {
		name := "missing"
		wantDays := int64(0)
		if count != nil {
			name, wantDays = "measured zero", 1
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "archive.sqlite")
			writer, err := store.OpenWriter(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := writer.InsertObs(t.Context(), 456, []model.DeviceObs{{Epoch: 1700000000, StrikeCount: count}}); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			st, err := store.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			closeOnCleanup(t, st)
			report, err := gatherStats(t.Context(), st, 1700000000, 1700000000)
			if err != nil {
				t.Fatal(err)
			}
			var output strings.Builder
			if err := writeStats(&output, report); err != nil {
				t.Fatal(err)
			}
			text := output.String()
			if strings.Contains(text, "none detected") || !strings.Contains(text, "no nonzero strike counts recorded") || !strings.Contains(text, "missing readings end runs") || report.light.LongestStormFreeDays != wantDays {
				t.Fatalf("lightning coverage was overstated: stats=%+v\n%s", report.light, text)
			}
		})
	}
}
