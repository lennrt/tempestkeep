package main

// `tempestkeep stats` prints a one-shot climate summary of the local archive: its
// coverage, all-time records, the warming/cooling trend, and the rain, wind,
// lightning, solar, and comfort highlights over a date range. It is the CLI
// surface for the analytics the MCP server exposes as tools, rendered as plain
// text so it pipes cleanly. Read-only: it opens the query_only store and never
// touches the network.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/config"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

// statsReport bundles the archive analytics that `tempestkeep stats` renders, all in
// US display units (the store converts at the edge).
type statsReport struct {
	cov      store.Coverage
	rec      store.Records
	trend    store.TempTrend
	rain     store.RainStats
	wind     store.WindStats
	light    store.LightningStats
	solar    store.SolarStats
	comfort  store.ComfortStats
	spells   store.TempSpells
	pressure store.PressureStats
}

func cmdStats(args []string) (err error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	describe(fs, "tempestkeep stats: one-shot climate summary of the archive (coverage, records,\ntrend, and rain/wind/lightning/solar/comfort highlights). --format json for scripting.",
		"tempestkeep stats",
		"tempestkeep stats --start 2024-01-01 --end 2024-12-31",
		"tempestkeep stats --format json | jq .records")
	db := fs.String("db", "", "path to the tempest.sqlite archive (or env TEMPEST_DB)")
	start := fs.String("start", "", "start date YYYY-MM-DD in local time (default: whole archive)")
	end := fs.String("end", "", "end date YYYY-MM-DD in local time, inclusive (default: today)")
	format := fs.String("format", "text", "output format: text or json")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	*format = strings.ToLower(*format)
	if *format != "text" && *format != "json" {
		return usagef("--format must be text or json")
	}

	startEpoch, endEpoch, err := exportRange(*start, *end)
	if err != nil {
		return usageErr{err}
	}

	if err := config.LoadDotenv(ctx, ".env"); err != nil {
		return err
	}
	dbPath, err := config.ResolveDB(ctx, *db)
	if err != nil {
		return err
	}
	if dbPath == "" {
		return fmt.Errorf("no archive configured: set --db/TEMPEST_DB, or run `tempestkeep setup`")
	}
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, st.Close()) }()

	rep, err := gatherStats(ctx, st, startEpoch, endEpoch)
	if err != nil {
		return err
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { err = errors.Join(err, out.Flush()) }()
	if *format == "json" {
		return writeStatsJSON(out, rep)
	}
	return writeStats(out, rep)
}

// statsJSON is the machine-readable shape of the report: the store analytics
// (already US-unit and json-tagged) grouped under stable keys, so `tempestkeep stats
// --format json | jq` is a first-class path alongside the human text.
type statsJSON struct {
	Coverage struct {
		Observations int64  `json:"observations"`
		From         string `json:"from,omitempty"`
		To           string `json:"to,omitempty"`
	} `json:"coverage"`
	Records           store.Records        `json:"records"`
	TemperatureTrend  store.TempTrend      `json:"temperature_trend"`
	Rain              store.RainStats      `json:"rain"`
	Wind              store.WindStats      `json:"wind"`
	Lightning         store.LightningStats `json:"lightning"`
	Solar             store.SolarStats     `json:"solar"`
	Pressure          store.PressureStats  `json:"pressure"`
	Comfort           store.ComfortStats   `json:"comfort"`
	TemperatureSpells store.TempSpells     `json:"temperature_spells"`
}

func writeStatsJSON(w io.Writer, r statsReport) error {
	var j statsJSON
	j.Coverage.Observations = r.cov.Count
	if r.cov.MinEpoch.Valid {
		j.Coverage.From = time.Unix(r.cov.MinEpoch.Int64, 0).Local().Format("2006-01-02")
		j.Coverage.To = time.Unix(r.cov.MaxEpoch.Int64, 0).Local().Format("2006-01-02")
	}
	j.Records, j.TemperatureTrend = r.rec, r.trend
	j.Rain, j.Wind, j.Lightning = r.rain, r.wind, r.light
	j.Solar, j.Pressure, j.Comfort = r.solar, r.pressure, r.comfort
	j.TemperatureSpells = r.spells

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(j)
}

// gatherStats runs the analytics over [start, end]. A zero start means the whole
// archive; the range-scoped sections (rain, wind, lightning, solar, comfort,
// spells) honor it, while records and the trend are always all-time.
func gatherStats(ctx context.Context, st *store.Store, start, end int64) (statsReport, error) {
	var r statsReport
	var err error
	if r.cov, err = st.Coverage(ctx); err != nil {
		return r, err
	}
	if r.rec, err = st.Records(ctx); err != nil {
		return r, err
	}
	if r.trend, err = st.TemperatureTrend(ctx); err != nil {
		return r, err
	}
	if r.rain, err = st.RainStats(ctx, start, end); err != nil {
		return r, err
	}
	if r.wind, err = st.WindStatistics(ctx, start, end); err != nil {
		return r, err
	}
	if r.light, err = st.LightningActivity(ctx, start, end); err != nil {
		return r, err
	}
	if r.solar, err = st.SolarActivity(ctx, start, end); err != nil {
		return r, err
	}
	if r.comfort, err = st.ComfortStatistics(ctx, start, end); err != nil {
		return r, err
	}
	if r.spells, err = st.TemperatureSpells(ctx, store.TempSpellParams{}, start, end); err != nil {
		return r, err
	}
	if r.pressure, err = st.PressureStatistics(ctx, start, end); err != nil {
		return r, err
	}
	return r, nil
}

// writeStats renders the report as plain text. Missing values (empty archive, a
// sensor that never reported) are shown as "—" rather than a misleading zero.
func writeStats(w io.Writer, r statsReport) error {
	out := textOutput{writer: w}
	dash := "—"
	f1 := func(p *float64, unit string) string {
		if p == nil {
			return dash
		}
		return fmt.Sprintf("%.1f%s", *p, unit)
	}
	epochDay := func(p *int64) string {
		if p == nil {
			return dash
		}
		return time.Unix(*p, 0).Local().Format("2006-01-02")
	}

	out.println("TempestKeep archive summary")
	out.println("===========================")
	if r.cov.Count == 0 || !r.cov.MinEpoch.Valid {
		out.println("The archive is empty. Run `tempestkeep collect` to build it.")
		return out.err
	}
	span := fmt.Sprintf("%s to %s",
		time.Unix(r.cov.MinEpoch.Int64, 0).Local().Format("2006-01-02"),
		time.Unix(r.cov.MaxEpoch.Int64, 0).Local().Format("2006-01-02"))
	out.printf("Coverage:   %d observations, %s\n", r.cov.Count, span)

	out.println("\nAll-time records")
	out.printf("  Hottest:    %s  (%s)\n", f1(r.rec.HottestF, "°F"), epochDay(r.rec.HottestEpoch))
	out.printf("  Coldest:    %s  (%s)\n", f1(r.rec.ColdestF, "°F"), epochDay(r.rec.ColdestEpoch))
	out.printf("  Peak gust:  %s  (%s)\n", f1(r.rec.PeakGustMph, " mph"), epochDay(r.rec.PeakGustEpoch))
	if r.rec.WettestDay != "" {
		out.printf("  Wettest:    %s  (%s)\n", f1(r.rec.WettestDayIn, " in"), r.rec.WettestDay)
	}

	out.println("\nTemperature trend")
	if r.trend.SlopePerDecadeF != nil {
		dir := "warming"
		if *r.trend.SlopePerDecadeF < 0 {
			dir = "cooling"
		}
		r2 := ""
		if r.trend.RSquared != nil {
			r2 = fmt.Sprintf(", R²=%.2f", *r.trend.RSquared)
		}
		out.printf("  %+.1f °F/decade (%s%s), over %d years\n", *r.trend.SlopePerDecadeF, dir, r2, r.trend.Years)
	} else {
		out.printf("  not enough history for a trend (%d year(s))\n", r.trend.Years)
	}

	out.println("\nRain")
	out.printf("  Total: %.2f in over %d days, %d rainy\n", r.rain.TotalIn, r.rain.DaysObserved, r.rain.RainyDays)
	if r.rain.LongestDrySpellDays > 0 {
		out.printf("  Longest dry spell: %d days (%s to %s)\n", r.rain.LongestDrySpellDays, r.rain.DrySpellStart, r.rain.DrySpellEnd)
	}

	out.println("\nWind")
	out.printf("  Average: %s, peak gust %s (%s), calm %.0f%%\n",
		f1(r.wind.AvgWindMph, " mph"), f1(r.wind.PeakGustMph, " mph"), r.wind.PeakGustDay, r.wind.CalmPct)

	out.println("\nLightning")
	if r.light.TotalStrikes > 0 {
		out.printf("  %d strikes over %d storm days; closest %s (%s)\n",
			r.light.TotalStrikes, r.light.StormDays, f1(r.light.ClosestStrikeMi, " mi"), r.light.ClosestStrikeDay)
	} else {
		out.printf("  none detected; %d storm-free days\n", r.light.LongestStormFreeDays)
	}

	out.println("\nSolar")
	out.printf("  Peak %s, peak UV %s; sunniest day %s (%s)\n",
		f1(r.solar.PeakSolarWm2, " W/m²"), f1(r.solar.PeakUV, ""), f1(r.solar.SunniestDayMJ, " MJ/m²"), r.solar.SunniestDay)

	out.println("\nPressure")
	out.printf("  Mean %s, range %s to %s\n",
		f1(r.pressure.MeanInHg, " inHg"), f1(r.pressure.LowestInHg, ""), f1(r.pressure.HighestInHg, " inHg"))
	if r.pressure.LargestFallInHg != nil {
		// The stored value is a signed delta (negative for a fall); "fall" already
		// carries the direction, so print the magnitude.
		mag := -*r.pressure.LargestFallInHg
		out.printf("  Largest daily fall: %.1f inHg (%s)\n", mag, r.pressure.LargestFallDay)
	}

	out.println("\nComfort extremes")
	out.printf("  Hottest feels-like: %s (%s), coldest: %s (%s)\n",
		f1(r.comfort.HottestFeelsLikeF, "°F"), r.comfort.HottestFeelsLikeDay,
		f1(r.comfort.ColdestFeelsLikeF, "°F"), r.comfort.ColdestFeelsLikeDay)
	if r.spells.LongestHeatWaveDays > 0 {
		out.printf("  Longest heat wave: %d days (%s to %s)\n", r.spells.LongestHeatWaveDays, r.spells.HeatWaveStart, r.spells.HeatWaveEnd)
	}
	if r.spells.LongestColdSnapDays > 0 {
		out.printf("  Longest cold snap: %d days (%s to %s)\n", r.spells.LongestColdSnapDays, r.spells.ColdSnapStart, r.spells.ColdSnapEnd)
	}
	return out.err
}
