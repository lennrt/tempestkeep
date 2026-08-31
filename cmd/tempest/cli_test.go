package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
)

func TestStripNoColor(t *testing.T) {
	cases := []struct {
		in       []string
		wantRest []string
		wantFlag bool
	}{
		{[]string{"now", "--once"}, []string{"now", "--once"}, false},
		{[]string{"now", "--no-color", "--once"}, []string{"now", "--once"}, true},
		{[]string{"--no-color", "stats"}, []string{"stats"}, true}, // flag before the command
		{[]string{"now", "-no-color"}, []string{"now"}, true},      // single-dash form
	}
	for _, tc := range cases {
		rest, flag := stripNoColor(tc.in)
		if flag != tc.wantFlag {
			t.Errorf("stripNoColor(%v) flag = %v, want %v", tc.in, flag, tc.wantFlag)
		}
		if len(rest) != len(tc.wantRest) {
			t.Fatalf("stripNoColor(%v) rest = %v, want %v", tc.in, rest, tc.wantRest)
		}
		for i := range rest {
			if rest[i] != tc.wantRest[i] {
				t.Errorf("stripNoColor(%v) rest = %v, want %v", tc.in, rest, tc.wantRest)
			}
		}
	}
}

func TestPrintFlagDefaultsUsesShortFlagFormAndWraps(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Bool("q", false, "short flag")
	fs.String("long-option-name", "", strings.Repeat("word ", 30))
	var buf bytes.Buffer
	out := textOutput{writer: &buf}
	printFlagDefaults(&out, fs)
	if out.err != nil {
		t.Fatal(out.err)
	}
	text := buf.String()
	if !strings.Contains(text, "  -q") || strings.Contains(text, "  --q") {
		t.Fatalf("short flag form is wrong:\n%s", text)
	}
	for line := range strings.Lines(text) {
		if len(line) > 96 {
			t.Fatalf("help line is %d bytes, want at most 96:\n%s", len(line), line)
		}
	}
}

func TestRootREADMEListsEveryTopLevelCommand(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"help", "version"}
	for name := range commands {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if !strings.Contains(string(data), "tempest "+name) {
			t.Errorf("root README does not list the %q command", name)
		}
	}
}

func TestWriteNowJSON(t *testing.T) {
	d := dashboard{
		station: "Backyard", source: "archive",
		obsTime:    time.Unix(1700000000, 0),
		conditions: "Clear",
		tempF:      new(72.5), windMph: new(float64(8)), windDirDeg: new(float64(90)), // 90° -> E
	}
	var buf bytes.Buffer
	if err := writeNowJSON(&buf, d); err != nil {
		t.Fatalf("writeNowJSON: %v", err)
	}
	var out nowJSON
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	if out.Source != "archive" || out.Station != "Backyard" {
		t.Errorf("header = %+v, want archive/Backyard", out)
	}
	if out.TempF == nil || *out.TempF != 72.5 {
		t.Errorf("temp_f = %v, want 72.5", out.TempF)
	}
	if out.WindDir != "E" {
		t.Errorf("wind_dir = %q, want E (derived from 90°)", out.WindDir)
	}
	// Absent sensors must be omitted, not rendered as null-ish zeros.
	if out.HumidityPct != nil {
		t.Errorf("humidity_pct = %v, want omitted", out.HumidityPct)
	}
}

func TestWriteDevicesJSON(t *testing.T) {
	stations := []api.Station{{
		StationID: 42, Name: "Home", Timezone: "UTC",
		Devices: []api.Device{{DeviceID: 7, DeviceType: "ST", SerialNumber: "ST-007"}},
	}}
	var buf bytes.Buffer
	if err := writeDevicesJSON(&buf, stations); err != nil {
		t.Fatalf("writeDevicesJSON: %v", err)
	}
	var out []stationJSON
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	if len(out) != 1 || out[0].StationID != 42 || len(out[0].Devices) != 1 {
		t.Fatalf("out = %+v, want one station with one device", out)
	}
	if out[0].Devices[0].Serial != "ST-007" || out[0].Devices[0].Type != "ST" {
		t.Errorf("device = %+v, want ST-007/ST", out[0].Devices[0])
	}
}

// TestCmdNowRejectsBadFormat pins the validation: an unknown --format is an
// error, not a silent fallback to text.
func TestCmdNowRejectsBadFormat(t *testing.T) {
	if err := cmdNow([]string{"--format", "yaml"}); err == nil {
		t.Error("cmdNow --format yaml should error")
	}
}

func TestCmdNowRejectsTooFastRefresh(t *testing.T) {
	if err := cmdNow([]string{"--interval", "4"}); err == nil || !isUsageErr(err) {
		t.Fatalf("cmdNow --interval 4 error = %v, want usage error", err)
	}
}

func TestCmdListDevicesRejectsBadFormat(t *testing.T) {
	if err := cmdListDevices([]string{"--format", "xml"}); err == nil {
		t.Error("cmdListDevices --format xml should error")
	}
}

func TestCmdCollectRejectsNegativeLimitsBeforeNetwork(t *testing.T) {
	t.Setenv("TEMPEST_TOKEN", "test")
	for _, args := range [][]string{
		{"--device-id=-1"},
		{"--backup-keep=-1"},
		{"--throttle-ms=-1"},
	} {
		if err := cmdCollect(args); err == nil || !isUsageErr(err) {
			t.Errorf("cmdCollect(%v) error = %v, want usage error", args, err)
		}
	}
}

func TestCmdCollectRejectsInvalidThrottleEnv(t *testing.T) {
	t.Setenv("TEMPEST_THROTTLE_MS", "fast")
	if err := cmdCollect(nil); err == nil || !strings.Contains(err.Error(), "TEMPEST_THROTTLE_MS") {
		t.Fatalf("cmdCollect invalid throttle error = %v", err)
	}
}

func TestClosestCommand(t *testing.T) {
	cases := map[string]string{
		"colect":       "collect",
		"exlore":       "explore",
		"stat":         "stats",
		"list-device":  "list-devices",
		"nwo":          "now",
		"definitely-x": "", // nothing plausible: fall back to full usage
	}
	for in, want := range cases {
		if got := closestCommand(in); got != want {
			t.Errorf("closestCommand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUsageErrClassification(t *testing.T) {
	// Bad invocations exit 2; runtime failures exit 1. main keys that split
	// off usageErr, so wrapping must survive an fmt.Errorf chain.
	ue := usagef("invalid --format %q", "yaml")
	if !isUsageErr(ue) {
		t.Error("usagef result not classified as usage error")
	}
	if !isUsageErr(fmt.Errorf("context: %w", ue)) {
		t.Error("wrapped usage error lost its classification")
	}
	if isUsageErr(errors.New("network died")) {
		t.Error("runtime error misclassified as usage error")
	}
}
