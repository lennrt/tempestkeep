package store_test

import (
	"math"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func trendObservation(year int, month time.Month, temperatureC float64) obsRow {
	return obsRow{epoch: localNoon(year, month, 15), tempC: new(temperatureC)}
}

func requireTrendFit(t *testing.T, got store.TempTrend, slope, rSquared float64) {
	t.Helper()
	if got.SlopePerDecadeF == nil {
		t.Fatalf("slope is absent, want %.12g°F/decade", slope)
	}
	if math.IsNaN(*got.SlopePerDecadeF) || math.IsInf(*got.SlopePerDecadeF, 0) || math.Abs(*got.SlopePerDecadeF-slope) > 1e-8 {
		t.Fatalf("slope = %.12g, want %.12g°F/decade", *got.SlopePerDecadeF, slope)
	}
	if got.RSquared == nil {
		t.Fatalf("R² is absent, want %.12g", rSquared)
	}
	if math.IsNaN(*got.RSquared) || *got.RSquared < 0 || *got.RSquared > 1 || math.Abs(*got.RSquared-rSquared) > 1e-10 {
		t.Fatalf("R² = %.12g, want %.12g within [0,1]", *got.RSquared, rSquared)
	}
}

func TestTemperatureTrendUnmatchedMonthCannotDiluteSlope(t *testing.T) {
	for _, changeC := range []float64{2, -2} {
		name := "warming"
		if changeC < 0 {
			name = "cooling"
		}
		t.Run(name, func(t *testing.T) {
			rows := []obsRow{
				trendObservation(2020, time.January, 0),
				trendObservation(2021, time.January, changeC),
			}
			base, err := openStoreWith(t, rows).TemperatureTrend(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			rows = append(rows, trendObservation(2025, time.December, 50))
			extended, err := openStoreWith(t, rows).TemperatureTrend(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			requireTrendFit(t, extended, changeC*18, 1)
			requireTrendFit(t, base, changeC*18, 1)
			if *extended.SlopePerDecadeF != *base.SlopePerDecadeF || *extended.RSquared != *base.RSquared {
				t.Fatalf("unmatched month changed trend: base=%+v, extended=%+v", base, extended)
			}
			if extended.Years != 3 || extended.MonthsUsed != 3 || extended.FirstMonth != "2020-01" || extended.LastMonth != "2025-12" {
				t.Fatalf("unmatched month must retain coverage: %+v", extended)
			}
			if *extended.WarmestAnomalyF != *base.WarmestAnomalyF || *extended.ColdestAnomalyF != *base.ColdestAnomalyF || extended.WarmestMonth != base.WarmestMonth || extended.ColdestMonth != base.ColdestMonth {
				t.Fatalf("unmatched zero anomaly changed extrema: base=%+v, extended=%+v", base, extended)
			}
		})
	}
}

func TestTemperatureTrendCentersUnevenMonthGroups(t *testing.T) {
	s := openStoreWith(t, []obsRow{
		trendObservation(2020, time.January, 0),
		trendObservation(2021, time.January, 2),
		trendObservation(2024, time.January, 8),
		trendObservation(2021, time.July, 22),
		trendObservation(2023, time.July, 26),
		trendObservation(2026, time.August, 30),
	})
	got, err := s.TemperatureTrend(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	requireTrendFit(t, got, 36, 1)
}

func TestTemperatureTrendWeightsMonthlyMeansEqually(t *testing.T) {
	rows := []obsRow{
		trendObservation(2020, time.January, 0),
		trendObservation(2021, time.January, 2),
		trendObservation(2020, time.July, 20),
		trendObservation(2024, time.July, 24),
	}
	// Monthly sample counts must not change the fit's equal monthly weights.
	for second := int64(1); second <= 100; second++ {
		row := trendObservation(2024, time.July, 24)
		row.epoch += second
		rows = append(rows, row)
	}
	got, err := openStoreWith(t, rows).TemperatureTrend(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// In Celsius, January contributes XX=.5, XY=1, YY=2. July contributes
	// XX=8, XY=8, YY=8. The pooled within-month fit is 9/8.5 °C/year.
	requireTrendFit(t, got, 324.0/17, 81.0/85)
	if got.MonthsUsed != 4 {
		t.Fatalf("sample count changed months used: %+v", got)
	}
}

func TestTemperatureTrendFlatRepeatedMonths(t *testing.T) {
	// Use fractional temperatures too: repeated identical means must not turn
	// a rounding residue into a nonzero trend or a defined correlation.
	var rows []obsRow
	for year := 2020; year <= 2025; year++ {
		rows = append(rows, trendObservation(year, time.January, .1), trendObservation(year, time.July, 25.1))
	}
	got, err := openStoreWith(t, rows).TemperatureTrend(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.SlopePerDecadeF == nil || *got.SlopePerDecadeF != 0 || got.RSquared != nil {
		t.Fatalf("flat repeated months need zero slope and absent R²: %+v", got)
	}
}

func TestTemperatureTrendIgnoresMissingTemperatures(t *testing.T) {
	rows := []obsRow{
		{epoch: localNoon(2019, time.January, 15)},
		trendObservation(2020, time.January, 0),
		trendObservation(2021, time.January, 2),
		{epoch: localNoon(2025, time.February, 15)},
	}
	got, err := openStoreWith(t, rows).TemperatureTrend(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	requireTrendFit(t, got, 36, 1)
	if got.MonthsUsed != 2 || got.Years != 2 || got.FirstMonth != "2020-01" || got.LastMonth != "2021-01" {
		t.Fatalf("missing temperatures changed usable coverage: %+v", got)
	}
	for _, samples := range [][]obsRow{
		nil,
		{{epoch: localNoon(2019, time.January, 15)}},
		{{epoch: localNoon(2019, time.January, 15)}, trendObservation(2020, time.January, 0), trendObservation(2021, time.July, 20)},
	} {
		unidentified, err := openStoreWith(t, samples).TemperatureTrend(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if unidentified.SlopePerDecadeF != nil || unidentified.RSquared != nil {
			t.Fatalf("no repeated usable month must leave slope/R² absent: %+v", unidentified)
		}
	}
}

func TestTemperatureTrendStableNearSupportedYearLimit(t *testing.T) {
	got, err := openStoreWith(t, []obsRow{
		trendObservation(9997, time.January, 0),
		trendObservation(9998, time.January, .00001),
		trendObservation(9999, time.January, .00002),
		trendObservation(9999, time.July, 25),
	}).TemperatureTrend(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	requireTrendFit(t, got, .00018, 1)
}
