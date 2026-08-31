package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

func TestWindArrow(t *testing.T) {
	// Meteorological "from" direction -> arrow pointing where the wind travels.
	cases := map[float64]string{
		0:   "↓", // from N, blows south
		90:  "←", // from E, blows west
		180: "↑", // from S, blows north
		270: "→", // from W, blows east
	}
	for from, want := range cases {
		if got := windArrow(from); got != want {
			t.Errorf("windArrow(%.0f) = %q, want %q", from, got, want)
		}
	}
	for _, from := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := windArrow(from); got != "•" {
			t.Errorf("windArrow(%v) = %q, want calm marker", from, got)
		}
	}
}

func TestTempColorRamp(t *testing.T) {
	// Cold reads blue-ish, hot reads red-ish, and the ramp never panics.
	if tempColor(0) != lipgloss.Color("#6FB1FF") {
		t.Errorf("freezing temp color = %v, want icy blue", tempColor(0))
	}
	if tempColor(100) != lipgloss.Color("#E63946") {
		t.Errorf("scorching temp color = %v, want red", tempColor(100))
	}
	if tempColor(70) != lipgloss.Color("#FFD23F") {
		t.Errorf("mild-warm temp color = %v, want yellow", tempColor(70))
	}
}

func sameArt(a, b weatherArt) bool {
	return a.accent == b.accent && strings.Join(a.lines, "\n") == strings.Join(b.lines, "\n")
}

func TestArtFor(t *testing.T) {
	cases := []struct {
		icon, conditions string
		want             weatherArt
	}{
		{"clear-day", "", artClearDay},
		{"clear-night", "", artClearNight},
		{"partly-cloudy-night", "", artPartlyNight},
		{"rainy", "", artRain},
		{"", "Thunderstorms likely", artThunder},
		{"snow", "", artSnow},
		{"totally-unknown-icon", "", artUnknown},
	}
	for _, tc := range cases {
		if got := artFor(tc.icon, tc.conditions); !sameArt(got, tc.want) {
			t.Errorf("artFor(%q,%q) accent=%s, want accent=%s", tc.icon, tc.conditions, got.accent, tc.want.accent)
		}
	}
}

// TestArtDimensions pins the invariant the dashboard columns rely on: every art
// block is exactly 5 rows, and every row is the same display width, so swapping
// or reshaping one picture can't silently misalign the layout.
func TestArtDimensions(t *testing.T) {
	all := map[string]weatherArt{
		"clear-day": artClearDay, "clear-night": artClearNight,
		"partly-day": artPartlyDay, "partly-night": artPartlyNight,
		"cloudy": artCloudy, "rain": artRain, "thunder": artThunder,
		"snow": artSnow, "sleet": artSleet, "fog": artFog,
		"wind": artWind, "unknown": artUnknown,
	}
	for name, a := range all {
		if len(a.lines) != 5 {
			t.Errorf("%s: %d lines, want 5", name, len(a.lines))
			continue
		}
		want := lipgloss.Width(a.lines[0])
		for i, line := range a.lines {
			if w := lipgloss.Width(line); w != want {
				t.Errorf("%s line %d width = %d, want %d (%q)", name, i, w, want, line)
			}
		}
	}
}

func TestGlyphWidthNormalized(t *testing.T) {
	// Every forecast glyph must render as two display columns after padding, so
	// the strip stays aligned regardless of emoji width quirks.
	for _, icon := range []string{"clear-day", "clear-night", "cloudy", "rainy", "snow", "windy", "unknown"} {
		g := padGlyph(glyphFor(icon, ""))
		if w := lipgloss.Width(g); w != 2 {
			t.Errorf("padGlyph(glyphFor(%q)) width = %d, want 2", icon, w)
		}
	}
}

func TestBuildArchiveDashboard(t *testing.T) {
	o := &model.Obs{Epoch: 1700000000, AirTempC: new(float64(25)), HumidityPct: new(float64(100)), WindAvgMps: new(float64(10)), WindDirDeg: new(float64(90)), RainMm: new(25.4)}
	d := buildArchiveDashboard("Backyard", o)

	if d.source != "archive" || d.station != "Backyard" {
		t.Errorf("header = %q/%q, want archive/Backyard", d.source, d.station)
	}
	if d.tempF == nil || math.Abs(*d.tempF-77) > 1e-6 { // 25°C
		t.Errorf("tempF = %v, want 77", d.tempF)
	}
	// Dew point at 100% RH ~ air temperature.
	if d.dewF == nil || math.Abs(*d.dewF-77) > 1.0 {
		t.Errorf("dewF = %v, want ~77", d.dewF)
	}
	if d.feelsF == nil {
		t.Error("archive dashboard did not derive feels-like")
	}
	if d.rainTodayIn != nil {
		t.Errorf("archive dashboard used one interval as today's rain: %v", d.rainTodayIn)
	}
	if d.windMph == nil || d.note == "" {
		t.Error("expected wind and an archive note")
	}
}

func TestRenderDashboardSmoke(t *testing.T) {
	// The renderer must produce a bordered card containing the key readings and
	// not panic on a mostly-populated dashboard.
	d := dashboard{
		station: "Smoke", source: "live", obsTime: time.Unix(1700000000, 0),
		conditions: "Clear", icon: "clear-day",
		tempF: new(float64(72)), feelsF: new(float64(74)), humidityPct: new(float64(40)), windMph: new(float64(5)), windDirDeg: new(float64(200)),
		daily: []dailyCell{{label: "Today", icon: glyphFor("clear-day", ""), hiF: new(float64(80)), loF: new(float64(55))}},
	}
	out := renderDashboard(d, time.Unix(1700000600, 0), "q quit")
	for _, want := range []string{"Smoke", "Clear", "72°F", "╭", "╰", "Today"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered card missing %q\n%s", want, out)
		}
	}
}

func TestHumanizeAge(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Second: "30s",
		5 * time.Minute:  "5m",
		3 * time.Hour:    "3h",
		50 * time.Hour:   "2d",
	}
	for d, want := range cases {
		if got := humanizeAge(d); got != want {
			t.Errorf("humanizeAge(%s) = %q, want %q", d, got, want)
		}
	}
}
