package main

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func TestWriteStatsEmptyArchive(t *testing.T) {
	var b strings.Builder
	if err := writeStats(&b, statsReport{}); err != nil { // zero coverage
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "archive is empty") {
		t.Errorf("empty archive should say so; got:\n%s", b.String())
	}
}

func TestWriteStatsRendersSections(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	rep := statsReport{
		cov:      store.Coverage{Count: 1000, MinEpoch: sql.NullInt64{Int64: 1700000000, Valid: true}, MaxEpoch: sql.NullInt64{Int64: 1700600000, Valid: true}},
		rec:      store.Records{HottestF: f(99.5), ColdestF: f(20.1), PeakGustMph: f(41.2)},
		trend:    store.TempTrend{Years: 3, SlopePerDecadeF: f(2.5), RSquared: f(0.8)},
		rain:     store.RainStats{TotalIn: 12.3, DaysObserved: 90, RainyDays: 14, LongestDrySpellDays: 8, DrySpellStart: "2024-07-01", DrySpellEnd: "2024-07-08"},
		wind:     store.WindStats{AvgWindMph: f(4.1), PeakGustMph: f(41.2), PeakGustDay: "2024-05-27", CalmPct: 33},
		light:    store.LightningStats{TotalStrikes: 42, StormDays: 3, ClosestStrikeMi: f(1.9), ClosestStrikeDay: "2024-08-04"},
		solar:    store.SolarStats{PeakSolarWm2: f(1100), PeakUV: f(9), SunniestDayMJ: f(28.5), SunniestDay: "2024-06-21"},
		comfort:  store.ComfortStats{HottestFeelsLikeF: f(105), HottestFeelsLikeDay: "2024-06-12", ColdestFeelsLikeF: f(10), ColdestFeelsLikeDay: "2024-01-15"},
		spells:   store.TempSpells{LongestHeatWaveDays: 4, HeatWaveStart: "2024-06-10", HeatWaveEnd: "2024-06-13"},
		pressure: store.PressureStats{MeanInHg: f(29.9), LowestInHg: f(29.1), HighestInHg: f(30.4), LargestFallInHg: f(-0.5), LargestFallDay: "2024-06-14"},
	}
	var b strings.Builder
	if err := writeStats(&b, rep); err != nil {
		t.Fatal(err)
	}
	out := b.String()

	for _, want := range []string{
		"1000 observations",
		"99.5°F", // hottest
		"+2.5 °F/decade (warming",
		"R²=0.80",
		"12.30 in over 90 days",
		"Longest dry spell: 8 days",
		"calm 33%",
		"42 strikes over 3 storm days",
		"closest 1.9 mi",
		"peak UV 9.0",
		"Hottest feels-like: 105.0°F",
		"Longest heat wave: 4 days",
		"Mean 29.9 inHg",
		"Largest daily fall: 0.5 inHg",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stats output missing %q; got:\n%s", want, out)
		}
	}
}

func TestWriteStatsJSON(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	rep := statsReport{
		cov:  store.Coverage{Count: 500, MinEpoch: sql.NullInt64{Int64: 1700000000, Valid: true}, MaxEpoch: sql.NullInt64{Int64: 1700600000, Valid: true}},
		rec:  store.Records{HottestF: f(99.5)},
		wind: store.WindStats{AvgWindMph: f(4.1), CalmPct: 33},
	}
	var b strings.Builder
	if err := writeStatsJSON(&b, rep); err != nil {
		t.Fatal(err)
	}
	var j statsJSON
	if err := json.Unmarshal([]byte(b.String()), &j); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, b.String())
	}
	if j.Coverage.Observations != 500 {
		t.Errorf("coverage.observations = %d, want 500", j.Coverage.Observations)
	}
	if j.Records.HottestF == nil || *j.Records.HottestF != 99.5 {
		t.Errorf("records.hottest_f = %v, want 99.5", j.Records.HottestF)
	}
	if j.Wind.AvgWindMph == nil || *j.Wind.AvgWindMph != 4.1 {
		t.Errorf("wind.avg_wind_mph = %v, want 4.1", j.Wind.AvgWindMph)
	}
}

func TestStatsRejectsBadFormat(t *testing.T) {
	if err := cmdStats([]string{"--format", "yaml", "--db", "nope.sqlite"}); err == nil {
		t.Error("expected an error for an unknown --format")
	}
}

func TestWriteStatsCoolingTrend(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	rep := statsReport{
		cov:   store.Coverage{Count: 10, MinEpoch: sql.NullInt64{Int64: 1700000000, Valid: true}, MaxEpoch: sql.NullInt64{Int64: 1700600000, Valid: true}},
		trend: store.TempTrend{Years: 4, SlopePerDecadeF: f(-1.2)},
		light: store.LightningStats{LongestStormFreeDays: 10},
	}
	var b strings.Builder
	if err := writeStats(&b, rep); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "-1.2 °F/decade (cooling") {
		t.Errorf("expected a cooling trend line; got:\n%s", out)
	}
	if !strings.Contains(out, "none detected") {
		t.Errorf("expected the no-lightning line; got:\n%s", out)
	}
}
