package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func TestReportRangesRejectUnsupportedBoundsBeforeIO(t *testing.T) {
	previous := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = previous })
	t.Chdir(t.TempDir())
	// If a command reads configuration first, this malformed file masks the
	// range error. Neither report needs to inspect configuration for bad dates.
	if err := os.WriteFile(".env", []byte("not a dotenv assignment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "missing", "archive.sqlite")
	for _, command := range []string{"stats", "export"} {
		for _, dates := range [][2]string{{"1900-01-01", "1900-01-02"}, {"0001-01-01", "9999-12-31"}, {"1970-01-01", "2100-01-01"}} {
			t.Run(command+"/"+dates[0]+"/"+dates[1], func(t *testing.T) {
				var stderr bytes.Buffer
				code := run([]string{command, "--db", path, "--start", dates[0], "--end", dates[1]}, io.Discard, &stderr)
				if code != exitUsage || !strings.Contains(stderr.String(), "--start") || strings.Contains(stderr.String(), "dotenv") {
					t.Fatalf("invalid range: status=%d, diagnostic=%q", code, stderr.String())
				}
				if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("range validation touched archive parent: %v", err)
				}
			})
		}
	}
}

func TestReportRangeSupportedBoundaries(t *testing.T) {
	previous := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = previous })
	start, end, err := exportRange("1970-01-01", "1970-01-01")
	if err != nil || start != 0 || end != 86399 {
		t.Fatalf("first supported day = %d..%d, %v", start, end, err)
	}
	start, end, err = exportRange("9999-12-31", "9999-12-31")
	if err != nil || end != model.MaxEpochSeconds || end-start != 86399 {
		t.Fatalf("last supported day = %d..%d, %v", start, end, err)
	}
	// The limit is elapsed seconds, not a count of calendar years. Test the
	// largest inclusive whole-day range and one additional day.
	lastDay := time.Unix(store.MaxRangeSeconds-1, 0).UTC().Format("2006-01-02")
	_, end, err = exportRange("1970-01-01", lastDay)
	if err != nil || end != store.MaxRangeSeconds-1 {
		t.Fatalf("last full day in span limit = %d, %v", end, err)
	}
	tooLate := time.Unix(store.MaxRangeSeconds, 0).UTC().Format("2006-01-02")
	if _, _, err := exportRange("1970-01-01", tooLate); err == nil {
		t.Fatal("inclusive end day beyond span limit accepted")
	}
	// Local calendar dates can fall outside supported UTC epochs.
	time.Local = time.FixedZone("synthetic-east", 3600)
	if _, _, err := exportRange("1970-01-01", "1970-01-01"); err == nil {
		t.Fatal("local start before epoch accepted")
	}
	time.Local = time.FixedZone("synthetic-west", -3600)
	if _, _, err := exportRange("9999-12-31", "9999-12-31"); err == nil {
		t.Fatal("local end beyond supported epoch accepted")
	}
}

func TestReportRangePreservesDSTCalendarDays(t *testing.T) {
	zone, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	previous := time.Local
	time.Local = zone
	t.Cleanup(func() { time.Local = previous })
	for _, tc := range []struct {
		date  string
		hours int64
	}{{"2024-03-10", 23}, {"2024-11-03", 25}} {
		start, end, err := exportRange(tc.date, tc.date)
		if err != nil || end-start+1 != tc.hours*3600 {
			t.Errorf("%s range = %d..%d, %v; want %d hours", tc.date, start, end, err, tc.hours)
		}
	}
}
