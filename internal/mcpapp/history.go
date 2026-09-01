package mcpapp

// This file defines read-only archive analysis tools.

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- input/output types -------------------------------------------------------

// ThisDayArgs is the this_day_in_history input.
type ThisDayArgs struct {
	Date string `json:"date,omitempty" jsonschema:"calendar day as MM-DD or YYYY-MM-DD (the year is ignored); defaults to today"`
}

// ThisDayOut is the this_day_in_history output: that calendar day in every year
// the archive covers, plus the records across those years.
type ThisDayOut struct {
	MonthDay       string          `json:"month_day"` // MM-DD
	Years          []store.YearDay `json:"years"`
	RecordHighF    *float64        `json:"record_high_f,omitempty"`
	RecordHighYear int             `json:"record_high_year,omitempty"`
	RecordLowF     *float64        `json:"record_low_f,omitempty"`
	RecordLowYear  int             `json:"record_low_year,omitempty"`
	WettestYear    int             `json:"wettest_year,omitempty"`
	WettestRainIn  *float64        `json:"wettest_rain_in,omitempty"`
	Note           string          `json:"note,omitempty"`
}

// PeriodSummaryArgs is the period_summary input.
type PeriodSummaryArgs struct {
	Period string `json:"period,omitempty" jsonschema:"bucket size: 'month' (default) or 'year'"`
	Start  string `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to the whole archive"`
	End    string `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
}

// PeriodSummaryOut is the period_summary output.
type PeriodSummaryOut struct {
	Period  string             `json:"period"` // "month" or "year"
	From    string             `json:"from,omitempty"`
	To      string             `json:"to,omitempty"`
	Periods []store.PeriodStat `json:"periods"`
	Note    string             `json:"note,omitempty"`
}

// WindRoseArgs is the wind_rose input.
type WindRoseArgs struct {
	Start string `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to the whole archive"`
	End   string `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
}

// WindRoseOut is the wind_rose output.
type WindRoseOut struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	store.WindRose
}

// GetObservationsArgs is the get_observations input.
type GetObservationsArgs struct {
	Start         string `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to 24 hours ago"`
	End           string `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to now"`
	BucketMinutes int    `json:"bucket_minutes,omitempty" jsonschema:"downsampling bucket width in minutes; defaults to the smallest width that keeps the series under max_points"`
	MaxPoints     int    `json:"max_points,omitempty" jsonschema:"cap on returned points when bucket_minutes is unset (default 288, max 2000)"`
}

// GetObservationsOut is the get_observations output: a downsampled time series.
type GetObservationsOut struct {
	From          string              `json:"from"`
	To            string              `json:"to"`
	BucketMinutes int                 `json:"bucket_minutes"`
	Points        []store.SeriesPoint `json:"points"`
	Note          string              `json:"note,omitempty"`
}

// DegreeDaysArgs is the degree_days input. Bases are in °F; zero means "use the
// convention default", so an agent can call it with no arguments.
type DegreeDaysArgs struct {
	Start        string  `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to the whole archive"`
	End          string  `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
	HeatingBaseF float64 `json:"heating_base_f,omitempty" jsonschema:"heating degree-day base in °F (default 65)"`
	CoolingBaseF float64 `json:"cooling_base_f,omitempty" jsonschema:"cooling degree-day base in °F (default 65)"`
	GrowingBaseF float64 `json:"growing_base_f,omitempty" jsonschema:"growing degree-day base in °F (default 50)"`
	GrowingCapF  float64 `json:"growing_cap_f,omitempty" jsonschema:"cap on the daily high for growing degree-days in °F (default 86); set negative to disable"`
	Monthly      bool    `json:"monthly,omitempty" jsonschema:"include the per-month breakdown (default false: range totals only)"`
}

// DegreeDaysOut is the degree_days output: the range total plus the bases used
// and, when requested, a per-month breakdown.
type DegreeDaysOut struct {
	From         string                `json:"from,omitempty"`
	To           string                `json:"to"`
	HeatingBaseF float64               `json:"heating_base_f"`
	CoolingBaseF float64               `json:"cooling_base_f"`
	GrowingBaseF float64               `json:"growing_base_f"`
	GrowingCapF  float64               `json:"growing_cap_f"`
	Total        store.DegreeDayStat   `json:"total"`
	Months       []store.DegreeDayStat `json:"months,omitempty"`
	Note         string                `json:"note,omitempty"`
}

// ClimateIndicesArgs is the climate_indices input. Thresholds are in °F; zero
// means "use the standard default".
type ClimateIndicesArgs struct {
	Start             string  `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to the whole archive"`
	End               string  `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
	FrostMaxF         float64 `json:"frost_max_f,omitempty" jsonschema:"frost day when the daily low is below this °F (default 32)"`
	IceMaxF           float64 `json:"ice_max_f,omitempty" jsonschema:"ice day when the daily high stays below this °F (default 32)"`
	SummerMinF        float64 `json:"summer_min_f,omitempty" jsonschema:"summer day when the daily high reaches this °F (default 77)"`
	HotMinF           float64 `json:"hot_min_f,omitempty" jsonschema:"hot day when the daily high reaches this °F (default 90)"`
	TropicalNightMinF float64 `json:"tropical_night_min_f,omitempty" jsonschema:"tropical night when the daily low stays at or above this °F (default 68)"`
	Yearly            bool    `json:"yearly,omitempty" jsonschema:"include the per-year breakdown (default false: range totals only)"`
}

// ClimateIndicesOut is the climate_indices output: threshold-day counts plus the
// thresholds used and, when requested, a per-year breakdown.
type ClimateIndicesOut struct {
	From              string                   `json:"from,omitempty"`
	To                string                   `json:"to"`
	FrostMaxF         float64                  `json:"frost_max_f"`
	IceMaxF           float64                  `json:"ice_max_f"`
	SummerMinF        float64                  `json:"summer_min_f"`
	HotMinF           float64                  `json:"hot_min_f"`
	TropicalNightMinF float64                  `json:"tropical_night_min_f"`
	Total             store.ClimateIndexStat   `json:"total"`
	Years             []store.ClimateIndexStat `json:"years,omitempty"`
	Note              string                   `json:"note,omitempty"`
}

// ClimateNormalsOut is the climate_normals output: the 12-month normal table.
type ClimateNormalsOut struct {
	Months []store.MonthNormal `json:"months"`
	Note   string              `json:"note,omitempty"`
}

// RainStatsArgs is the rain_stats input.
type RainStatsArgs struct {
	Start string `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to the whole archive"`
	End   string `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
}

// RainStatsOut is the rain_stats output.
type RainStatsOut struct {
	From string `json:"from,omitempty"`
	To   string `json:"to"`
	store.RainStats
	Note string `json:"note,omitempty"`
}

// PressureTrendArgs is the pressure_trend input.
type PressureTrendArgs struct {
	WindowHours float64 `json:"window_hours,omitempty" jsonschema:"trailing window for the tendency in hours (default 3, the meteorological standard)"`
}

// PressureTrendOut is the pressure_trend output.
type PressureTrendOut struct {
	store.PressureTrend
	Note string `json:"note,omitempty"`
}

// ClimatologyArgs is the climatology input.
type ClimatologyArgs struct {
	Start string `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to the whole archive"`
	End   string `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
}

// ClimatologyOut is the climatology output: the average day, hour by hour.
type ClimatologyOut struct {
	From  string           `json:"from,omitempty"`
	To    string           `json:"to"`
	Hours []store.HourStat `json:"hours"`
	Note  string           `json:"note,omitempty"`
}

// LightningArgs is the lightning_activity input.
type LightningArgs struct {
	Start string `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to the whole archive"`
	End   string `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
}

// LightningOut is the lightning_activity output.
type LightningOut struct {
	From string `json:"from,omitempty"`
	To   string `json:"to"`
	store.LightningStats
	Note string `json:"note,omitempty"`
}

// SolarArgs is the solar_stats input.
type SolarArgs struct {
	Start string `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to the whole archive"`
	End   string `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
}

// SolarOut is the solar_stats output.
type SolarOut struct {
	From string `json:"from,omitempty"`
	To   string `json:"to"`
	store.SolarStats
	Note string `json:"note,omitempty"`
}

// WindStatsArgs is the wind_stats input.
type WindStatsArgs struct {
	Start string `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to the whole archive"`
	End   string `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
}

// WindStatsOut is the wind_stats output.
type WindStatsOut struct {
	From string `json:"from,omitempty"`
	To   string `json:"to"`
	store.WindStats
	Note string `json:"note,omitempty"`
}

// ComfortArgs is the comfort_stats input.
type ComfortArgs struct {
	Start string `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to the whole archive"`
	End   string `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
}

// ComfortOut is the comfort_stats output.
type ComfortOut struct {
	From string `json:"from,omitempty"`
	To   string `json:"to"`
	store.ComfortStats
	Note string `json:"note,omitempty"`
}

// SensorHealthArgs is the sensor_health input.
type SensorHealthArgs struct {
	Start string `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to the whole archive"`
	End   string `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
}

// SensorHealthOut is the sensor_health output.
type SensorHealthOut struct {
	From string `json:"from,omitempty"`
	To   string `json:"to"`
	store.SensorHealth
	Note string `json:"note,omitempty"`
}

// PressureStatsArgs is the pressure_stats input.
type PressureStatsArgs struct {
	Start string `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to the whole archive"`
	End   string `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
}

// PressureStatsOut is the pressure_stats output.
type PressureStatsOut struct {
	From string `json:"from,omitempty"`
	To   string `json:"to"`
	store.PressureStats
	Note string `json:"note,omitempty"`
}

// TemperatureSpellsArgs is the temperature_spells input. Thresholds are in °F;
// zero means "use the standard default".
type TemperatureSpellsArgs struct {
	Start    string  `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; defaults to the whole archive"`
	End      string  `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
	HeatMinF float64 `json:"heat_min_f,omitempty" jsonschema:"heat-wave day when the daily high reaches this °F (default 90)"`
	ColdMaxF float64 `json:"cold_max_f,omitempty" jsonschema:"cold-snap day when the daily low reaches this °F (default 32)"`
}

// TemperatureSpellsOut is the temperature_spells output.
type TemperatureSpellsOut struct {
	From string `json:"from,omitempty"`
	To   string `json:"to"`
	store.TempSpells
	Note string `json:"note,omitempty"`
}

// TemperatureTrendOut is the temperature_trend output.
type TemperatureTrendOut struct {
	store.TempTrend
	Note string `json:"note,omitempty"`
}

// QuerySQLArgs is the query_sql input.
type QuerySQLArgs struct {
	SQL     string `json:"sql" jsonschema:"a single SELECT (or WITH … SELECT) statement over the obs_st table; see the tempest://archive/schema resource"`
	MaxRows int    `json:"max_rows,omitempty" jsonschema:"row cap (default 100, max 1000)"`
}

// QuerySQLOut is the query_sql output.
type QuerySQLOut struct {
	store.QueryResult
	Note string `json:"note,omitempty"`
}

// ---- registration ---------------------------------------------------------------

// registerHistoryTools adds every read-only archive tool.
func registerHistoryTools(srv *mcp.Server, st *store.Store) {
	registerCalendarTools(srv, st)
	registerClimateTools(srv, st)
	registerSensorTools(srv, st)
	registerTrendAndQueryTools(srv, st)
}

func registerCalendarTools(srv *mcp.Server, st *store.Store) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "this_day_in_history",
		Title:       "This day in history",
		Description: "Return this calendar day for each archived year. Results include temperature (°F), rain (in), gusts (mph), and cross-year records. date accepts MM-DD or YYYY-MM-DD and defaults to today.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args ThisDayArgs) (*mcp.CallToolResult, ThisDayOut, error) {
		month, day, err := parseMonthDay(args.Date)
		if err != nil {
			return nil, ThisDayOut{}, err
		}
		years, err := st.ThisDay(ctx, month, day)
		if err != nil {
			return nil, ThisDayOut{}, err
		}
		out := ThisDayOut{MonthDay: fmt.Sprintf("%02d-%02d", month, day), Years: years}
		for _, y := range years {
			if y.TempMaxF != nil && (out.RecordHighF == nil || *y.TempMaxF > *out.RecordHighF) {
				out.RecordHighF, out.RecordHighYear = y.TempMaxF, y.Year
			}
			if y.TempMinF != nil && (out.RecordLowF == nil || *y.TempMinF < *out.RecordLowF) {
				out.RecordLowF, out.RecordLowYear = y.TempMinF, y.Year
			}
			if y.RainIn > 0 && (out.WettestRainIn == nil || y.RainIn > *out.WettestRainIn) {
				rain := y.RainIn
				out.WettestRainIn, out.WettestYear = &rain, y.Year
			}
		}
		if len(years) == 0 {
			out.Note = "the archive has no observations on this calendar day"
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "period_summary",
		Title:       "Monthly / yearly summary",
		Description: "Group archive data by calendar month or year. Results include temperature (°F), rain (in), gusts (mph), observed days, and rainy days. The default range is the full archive.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args PeriodSummaryArgs) (*mcp.CallToolResult, PeriodSummaryOut, error) {
		period, err := parsePeriod(args.Period)
		if err != nil {
			return nil, PeriodSummaryOut{}, err
		}
		start, end, err := resolveOptionalRange(args.Start, args.End, 0)
		if err != nil {
			return nil, PeriodSummaryOut{}, err
		}
		periods, err := st.PeriodSummary(ctx, period, start, end)
		if err != nil {
			return nil, PeriodSummaryOut{}, err
		}
		out := PeriodSummaryOut{Period: periodName(period), To: localDate(end), Periods: periods}
		if start > 0 {
			// start == 0 means "whole archive": leave From empty rather than
			// reporting the 1970 epoch floor.
			out.From = localDate(start)
		}
		if len(periods) == 0 {
			out.Note = "no observations in this range"
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wind_rose",
		Title:       "Wind rose",
		Description: "Return wind distribution across 16 compass sectors. Each sector includes its share, mean speed, and peak gust in mph. The default range is the full archive.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args WindRoseArgs) (*mcp.CallToolResult, WindRoseOut, error) {
		start, end, err := resolveOptionalRange(args.Start, args.End, 0)
		if err != nil {
			return nil, WindRoseOut{}, err
		}
		rose, err := st.WindRose(ctx, start, end)
		if err != nil {
			return nil, WindRoseOut{}, err
		}
		// Report the resolved window (like daily_summary / get_observations),
		// not the raw input strings, so an agent that omitted dates still sees
		// what span "whole archive" covered. start == 0 means unbounded start.
		out := WindRoseOut{WindRose: rose, To: localDate(end)}
		if start > 0 {
			out.From = localDate(start)
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_observations",
		Title:       "Observation series (downsampled)",
		Description: "Return a bounded, downsampled observation series. Values use °F, inHg, mph, and inches. The default range is 24 hours. max_points defaults to 288 and cannot exceed 2000.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args GetObservationsArgs) (*mcp.CallToolResult, GetObservationsOut, error) {
		maxPoints, err := optionalBoundedInt(args.MaxPoints, 288, 2000, "max_points")
		if err != nil {
			return nil, GetObservationsOut{}, err
		}
		maxBucketMinutes := int(store.MaxRangeSeconds / 60)
		bucketMinutes, err := optionalBoundedInt(args.BucketMinutes, 0, maxBucketMinutes, "bucket_minutes")
		if err != nil {
			return nil, GetObservationsOut{}, err
		}
		now := time.Now()
		start, end, err := resolveOptionalRange(args.Start, args.End, now.Add(-24*time.Hour).Unix())
		if err != nil {
			return nil, GetObservationsOut{}, err
		}
		bucket := int64(bucketMinutes) * 60
		coarsened := false
		if bucket <= 0 {
			bucket = autoBucket(start, end, maxPoints)
		} else if minBucket := autoBucket(start, end, 2000); bucket < minBucket {
			// An explicit bucket must still respect the hard point cap, or a
			// small bucket over a multi-year range materializes millions of
			// points into one tool result.
			bucket = minBucket
			coarsened = true
		}
		points, err := st.Series(ctx, start, end, bucket)
		if err != nil {
			return nil, GetObservationsOut{}, err
		}
		out := GetObservationsOut{
			From: localTimeStr(start), To: localTimeStr(end),
			BucketMinutes: int(bucket / 60), Points: points,
		}
		switch {
		case len(points) == 0:
			out.Note = "no observations in this range"
		case coarsened:
			// Tell the caller its bucket_minutes was overridden, so the returned
			// resolution isn't silently coarser than what it asked for.
			out.Note = fmt.Sprintf("requested bucket_minutes was too fine for this range; widened to %d to stay under the point cap", bucket/60)
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "climate_normals",
		Title:       "Climate normals (12-month table)",
		Description: "Return archive averages for each calendar month. Results include temperature (°F), rain (in), rainy days, and the number of contributing years.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ NoArgs) (*mcp.CallToolResult, ClimateNormalsOut, error) {
		months, err := st.MonthlyNormals(ctx)
		if err != nil {
			return nil, ClimateNormalsOut{}, err
		}
		out := ClimateNormalsOut{Months: months}
		if len(months) == 0 {
			out.Note = "the archive has no observations yet"
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "rain_stats",
		Title:       "Rainfall statistics",
		Description: "Return rain totals, rainy days, the wettest day, and the longest dry and wet spells. Coverage gaps end a spell. The default range is the full archive.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args RainStatsArgs) (*mcp.CallToolResult, RainStatsOut, error) {
		start, end, err := resolveOptionalRange(args.Start, args.End, 0)
		if err != nil {
			return nil, RainStatsOut{}, err
		}
		rs, err := st.RainStats(ctx, start, end)
		if err != nil {
			return nil, RainStatsOut{}, err
		}
		out := RainStatsOut{To: localDate(end), RainStats: rs}
		if start > 0 {
			out.From = localDate(start)
		}
		if rs.DaysObserved == 0 {
			out.Note = "no observations in this range"
		}
		return nil, out, nil
	})
}

func registerClimateTools(srv *mcp.Server, st *store.Store) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "pressure_trend",
		Title:       "Barometric tendency",
		Description: "Return the change in station pressure over a trailing window. The default is 3 hours. The accepted range is 1 second through 24 hours.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args PressureTrendArgs) (*mcp.CallToolResult, PressureTrendOut, error) {
		windowHours := args.WindowHours
		if windowHours == 0 {
			windowHours = 3
		}
		windowSeconds := windowHours * 3600
		if math.IsNaN(windowSeconds) || math.IsInf(windowSeconds, 0) || windowSeconds < 1 || windowSeconds > 24*3600 {
			return nil, PressureTrendOut{}, fmt.Errorf("window_hours must represent 1 second through 24 hours")
		}
		window := int64(windowSeconds)
		trend, ok, err := st.PressureTendency(ctx, window)
		if err != nil {
			return nil, PressureTrendOut{}, err
		}
		if !ok {
			return nil, PressureTrendOut{Note: "not enough pressure history for a trend (need readings spanning at least half the window)"}, nil
		}
		return nil, PressureTrendOut{PressureTrend: *trend}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "climate_indices",
		Title:       "Climate threshold-day indices",
		Description: "Count frost, ice, summer, hot, and tropical-night days. Thresholds use °F and can be changed. The default range is the full archive. Set yearly to include annual counts.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args ClimateIndicesArgs) (*mcp.CallToolResult, ClimateIndicesOut, error) {
		start, end, err := resolveOptionalRange(args.Start, args.End, 0)
		if err != nil {
			return nil, ClimateIndicesOut{}, err
		}
		p := store.ClimateIndexParams{
			FrostMaxF: args.FrostMaxF, IceMaxF: args.IceMaxF, SummerMinF: args.SummerMinF,
			HotMinF: args.HotMinF, TropicalNightMinF: args.TropicalNightMinF,
		}
		total, years, err := st.ClimateIndices(ctx, p, start, end)
		if err != nil {
			return nil, ClimateIndicesOut{}, err
		}
		// Report the thresholds actually applied (the store fills the defaults).
		eff := p.WithDefaults()
		out := ClimateIndicesOut{
			To: localDate(end), Total: total,
			FrostMaxF: eff.FrostMaxF, IceMaxF: eff.IceMaxF, SummerMinF: eff.SummerMinF,
			HotMinF: eff.HotMinF, TropicalNightMinF: eff.TropicalNightMinF,
		}
		if start > 0 {
			out.From = localDate(start)
		}
		if args.Yearly {
			out.Years = years
		}
		if total.Days == 0 {
			out.Note = "no days with both a high and a low in this range"
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "climatology",
		Title:       "Typical day (hourly climatology)",
		Description: "Return conditions by local hour. Each hour includes temperature (°F), humidity, wind, and gusts (mph). The default range is the full archive.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args ClimatologyArgs) (*mcp.CallToolResult, ClimatologyOut, error) {
		start, end, err := resolveOptionalRange(args.Start, args.End, 0)
		if err != nil {
			return nil, ClimatologyOut{}, err
		}
		hours, err := st.HourlyClimatology(ctx, start, end)
		if err != nil {
			return nil, ClimatologyOut{}, err
		}
		out := ClimatologyOut{To: localDate(end), Hours: hours}
		if start > 0 {
			out.From = localDate(start)
		}
		if len(hours) == 0 {
			out.Note = "no observations in this range"
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "degree_days",
		Title:       "Degree-days (heating / cooling / growing)",
		Description: "Calculate heating, cooling, and growing degree-days in °F-days. Heating and cooling bases default to 65°F. The growing base defaults to 50°F and its cap to 86°F. Set monthly for monthly results.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args DegreeDaysArgs) (*mcp.CallToolResult, DegreeDaysOut, error) {
		start, end, err := resolveOptionalRange(args.Start, args.End, 0)
		if err != nil {
			return nil, DegreeDaysOut{}, err
		}
		p := store.DegreeDayParams{
			HeatingBaseF: orDefault(args.HeatingBaseF, 65),
			CoolingBaseF: orDefault(args.CoolingBaseF, 65),
			GrowingBaseF: orDefault(args.GrowingBaseF, 50),
			GrowingCapF:  orDefault(args.GrowingCapF, 86),
		}
		if args.GrowingCapF < 0 {
			p.GrowingCapF = 0 // caller opted out of capping
		}
		total, months, err := st.DegreeDays(ctx, p, start, end)
		if err != nil {
			return nil, DegreeDaysOut{}, err
		}
		out := DegreeDaysOut{
			To: localDate(end), Total: total,
			HeatingBaseF: p.HeatingBaseF, CoolingBaseF: p.CoolingBaseF,
			GrowingBaseF: p.GrowingBaseF, GrowingCapF: p.GrowingCapF,
		}
		if start > 0 {
			out.From = localDate(start)
		}
		if args.Monthly {
			out.Months = months
		}
		if total.Days == 0 {
			out.Note = "no days with both a high and a low in this range"
		}
		return nil, out, nil
	})
}

func registerSensorTools(srv *mcp.Server, st *store.Store) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "lightning_activity",
		Title:       "Lightning activity",
		Description: "Return strike counts, storm days, distance, busiest day, and storm-free spells. Distances use miles. Coverage gaps end a spell. The default range is the full archive.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args LightningArgs) (*mcp.CallToolResult, LightningOut, error) {
		start, end, err := resolveOptionalRange(args.Start, args.End, 0)
		if err != nil {
			return nil, LightningOut{}, err
		}
		ls, err := st.LightningActivity(ctx, start, end)
		if err != nil {
			return nil, LightningOut{}, err
		}
		out := LightningOut{To: localDate(end), LightningStats: ls}
		if start > 0 {
			out.From = localDate(start)
		}
		if ls.DaysObserved == 0 {
			out.Note = "no observations in this range"
		} else if ls.TotalStrikes == 0 {
			out.Note = "no lightning detected in this range"
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "solar_stats",
		Title:       "Solar and UV statistics",
		Description: "Return solar, UV, illuminance, and insolation statistics. Insolation covers observed time only. The default range is the full archive.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args SolarArgs) (*mcp.CallToolResult, SolarOut, error) {
		start, end, err := resolveOptionalRange(args.Start, args.End, 0)
		if err != nil {
			return nil, SolarOut{}, err
		}
		ss, err := st.SolarActivity(ctx, start, end)
		if err != nil {
			return nil, SolarOut{}, err
		}
		out := SolarOut{To: localDate(end), SolarStats: ss}
		if start > 0 {
			out.From = localDate(start)
		}
		if ss.DaysObserved == 0 {
			out.Note = "no observations in this range"
		} else if ss.PeakSolarWm2 == nil {
			out.Note = "no solar readings in this range"
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wind_stats",
		Title:       "Wind speed statistics",
		Description: "Return mean wind, peak gust, peak sustained wind, the windiest day, and calm share. Speeds use mph. The default range is the full archive.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args WindStatsArgs) (*mcp.CallToolResult, WindStatsOut, error) {
		start, end, err := resolveOptionalRange(args.Start, args.End, 0)
		if err != nil {
			return nil, WindStatsOut{}, err
		}
		ws, err := st.WindStatistics(ctx, start, end)
		if err != nil {
			return nil, WindStatsOut{}, err
		}
		out := WindStatsOut{To: localDate(end), WindStats: ws}
		if start > 0 {
			out.From = localDate(start)
		}
		if ws.Obs == 0 {
			out.Note = "no wind observations in this range"
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "comfort_stats",
		Title:       "Human-comfort extremes",
		Description: "Return heat-index, wind-chill, and dew-point extremes in °F. Calculations use matching 15-minute sensor means. The default range is the full archive.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args ComfortArgs) (*mcp.CallToolResult, ComfortOut, error) {
		start, end, err := resolveOptionalRange(args.Start, args.End, 0)
		if err != nil {
			return nil, ComfortOut{}, err
		}
		cs, err := st.ComfortStatistics(ctx, start, end)
		if err != nil {
			return nil, ComfortOut{}, err
		}
		out := ComfortOut{To: localDate(end), ComfortStats: cs}
		if start > 0 {
			out.From = localDate(start)
		}
		if cs.DaysObserved == 0 {
			// ComfortStatistics counts only days that carried a temperature reading,
			// the input every feels-like figure needs, so say so precisely.
			out.Note = "no temperature observations in this range"
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "sensor_health",
		Title:       "Sensor health diagnostic",
		Description: "Return count, coverage, last reading, and stale state for continuous sensors. A sensor is stale after one hour. Results also include the battery range. The default range is the full archive.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args SensorHealthArgs) (*mcp.CallToolResult, SensorHealthOut, error) {
		start, end, err := resolveOptionalRange(args.Start, args.End, 0)
		if err != nil {
			return nil, SensorHealthOut{}, err
		}
		h, err := st.SensorHealthReport(ctx, start, end)
		if err != nil {
			return nil, SensorHealthOut{}, err
		}
		out := SensorHealthOut{To: localDate(end), SensorHealth: h}
		if start > 0 {
			out.From = localDate(start)
		}
		if h.Observations == 0 {
			out.Note = "no observations in this range"
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "pressure_stats",
		Title:       "Barometric pressure statistics",
		Description: "Return station-pressure mean, extremes, and daily changes in inHg. Changes compare consecutive observed days only. The default range is the full archive.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args PressureStatsArgs) (*mcp.CallToolResult, PressureStatsOut, error) {
		start, end, err := resolveOptionalRange(args.Start, args.End, 0)
		if err != nil {
			return nil, PressureStatsOut{}, err
		}
		ps, err := st.PressureStatistics(ctx, start, end)
		if err != nil {
			return nil, PressureStatsOut{}, err
		}
		out := PressureStatsOut{To: localDate(end), PressureStats: ps}
		if start > 0 {
			out.From = localDate(start)
		}
		if ps.DaysObserved == 0 {
			out.Note = "no observations in this range"
		} else if ps.MeanInHg == nil {
			out.Note = "no pressure readings in this range"
		}
		return nil, out, nil
	})
}

func registerTrendAndQueryTools(srv *mcp.Server, st *store.Store) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "temperature_spells",
		Title:       "Heat waves and cold snaps",
		Description: "Return the longest heat wave and cold snap. Heat defaults to at least 90°F. Cold defaults to at most 32°F. Coverage gaps end a run.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args TemperatureSpellsArgs) (*mcp.CallToolResult, TemperatureSpellsOut, error) {
		start, end, err := resolveOptionalRange(args.Start, args.End, 0)
		if err != nil {
			return nil, TemperatureSpellsOut{}, err
		}
		ts, err := st.TemperatureSpells(ctx, store.TempSpellParams{HeatMinF: args.HeatMinF, ColdMaxF: args.ColdMaxF}, start, end)
		if err != nil {
			return nil, TemperatureSpellsOut{}, err
		}
		out := TemperatureSpellsOut{To: localDate(end), TempSpells: ts}
		if start > 0 {
			out.From = localDate(start)
		}
		if ts.DaysObserved == 0 {
			out.Note = "no observations in this range"
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "temperature_trend",
		Title:       "Warming / cooling trend",
		Description: "Fit a temperature trend from monthly anomalies across the full archive. Results include °F per decade, R², and the largest anomalies. A short archive returns no slope.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ NoArgs) (*mcp.CallToolResult, TemperatureTrendOut, error) {
		tt, err := st.TemperatureTrend(ctx)
		if err != nil {
			return nil, TemperatureTrendOut{}, err
		}
		out := TemperatureTrendOut{TempTrend: tt}
		switch {
		case tt.MonthsUsed == 0:
			out.Note = "the archive has no monthly temperature data yet"
		case tt.SlopePerDecadeF == nil:
			out.Note = "not enough spread across years to fit a trend (need the same calendar months observed in more than one year)"
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "query_sql",
		Title:       "Read-only SQL query",
		Description: "Run one SELECT or WITH SELECT against the read-only archive. Values use SI units. Query text is limited to 64 KiB. max_rows defaults to 100 and cannot exceed 1000.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args QuerySQLArgs) (*mcp.CallToolResult, QuerySQLOut, error) {
		maxRows, err := optionalBoundedInt(args.MaxRows, 100, store.MaxQueryRows, "max_rows")
		if err != nil {
			return nil, QuerySQLOut{}, err
		}
		// The store is pinned to one connection, so a runaway query (say a
		// recursive CTE) would wedge every other read tool behind it if the
		// client never cancels; bound it server-side.
		qctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		res, err := st.Query(qctx, args.SQL, maxRows)
		if err != nil {
			return nil, QuerySQLOut{}, err
		}
		out := QuerySQLOut{QueryResult: res, Note: "values are SI (°C, m/s, mb, mm, km); epoch is unix seconds UTC"}
		if res.Truncated {
			out.Note += fmt.Sprintf("; truncated at %d rows", maxRows)
		}
		return nil, out, nil
	})
}

// ---- helpers --------------------------------------------------------------------

// parseMonthDay accepts "", "MM-DD", or "YYYY-MM-DD" (year ignored) and returns
// the month and day, defaulting to today in local time.
func parseMonthDay(s string) (month, day int, err error) {
	if s == "" {
		now := time.Now()
		return int(now.Month()), now.Day(), nil
	}
	if len(s) == len("2006-01-02") {
		if t, parseErr := time.Parse("2006-01-02", s); parseErr == nil {
			return int(t.Month()), t.Day(), nil
		}
	}
	if len(s) == len("01-02") {
		if t, parseErr := time.Parse("01-02", s); parseErr == nil {
			return int(t.Month()), t.Day(), nil
		}
	}
	return 0, 0, fmt.Errorf("date must be MM-DD or YYYY-MM-DD")
}

// parsePeriod maps the tool argument to an archive period.
func parsePeriod(s string) (store.Period, error) {
	if len(s) > len("monthly") {
		return 0, fmt.Errorf("period must be month or year")
	}
	switch strings.ToLower(s) {
	case "", "month", "monthly":
		return store.PeriodMonth, nil
	case "year", "yearly", "annual":
		return store.PeriodYear, nil
	}
	return 0, fmt.Errorf("period must be month or year")
}

func periodName(period store.Period) string {
	if period == store.PeriodYear {
		return "year"
	}
	return "month"
}

// resolveOptionalRange turns optional local start/end dates into an inclusive
// epoch range. An empty start falls back to defStart (0 = the whole archive);
// an empty end means now. A start after the end is an error rather than a
// silently empty result.
func resolveOptionalRange(startDate, endDate string, defStart int64) (int64, int64, error) {
	start := defStart
	if startDate != "" {
		t, err := parseLocalDate(startDate)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid start: %w", err)
		}
		start = t.Unix()
	}
	end := time.Now().Unix()
	if endDate != "" {
		t, err := parseLocalDate(endDate)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid end: %w", err)
		}
		end = t.AddDate(0, 0, 1).Unix() - 1 // include the whole end day
		if startDate == "" && start > end {
			// end alone, older than the default window: the caller wants that
			// day, not an error about a default start they never supplied.
			start = t.Unix()
		}
	}
	if start > end {
		return 0, 0, fmt.Errorf("start date must not be after end date")
	}
	return start, end, nil
}

// autoBucket picks the smallest bucket width (≥1 minute) that keeps a series
// under maxPoints.
func autoBucket(start, end int64, maxPoints int) int64 {
	span := end - start
	if span <= 0 || maxPoints <= 0 {
		return 60
	}
	bucket := span / int64(maxPoints)
	if rem := span % int64(maxPoints); rem != 0 {
		bucket++
	}
	// Round up to a whole minute so bucket boundaries stay readable.
	if rem := bucket % 60; rem != 0 {
		bucket += 60 - rem
	}
	if bucket < 60 {
		bucket = 60
	}
	return bucket
}
