package main

// Integration tests for the analytics tools, resources, and prompts, driven
// over the real MCP protocol against a small archive, the same wire path an
// agent takes (see integration_test.go for the harness).

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestIntegrationHistoryTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cs := connectArchiveServer(t, ctx, makeTestArchive(t))

	// Every analytics tool must be listed, read-only annotated, and titled.
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tl := range tools.Tools {
		byName[tl.Name] = tl
	}
	for _, name := range []string{"this_day_in_history", "period_summary", "wind_rose", "climate_normals", "rain_stats", "pressure_trend", "climatology", "climate_indices", "degree_days", "lightning_activity", "solar_stats", "wind_stats", "comfort_stats", "temperature_trend", "temperature_spells", "pressure_stats", "sensor_health", "get_observations", "query_sql"} {
		tl, ok := byName[name]
		if !ok {
			t.Errorf("analytics tool %q not registered", name)
			continue
		}
		if tl.Annotations == nil || !tl.Annotations.ReadOnlyHint {
			t.Errorf("tool %q must carry readOnlyHint", name)
		}
		if tl.Title == "" {
			t.Errorf("tool %q has no title", name)
		}
		if tl.OutputSchema == nil {
			t.Errorf("tool %q has no output schema", name)
		}
	}

	// The fixture rows are at epochs 1700000000 and 1700086400 (local dates vary
	// by test host zone), so derive the expected calendar day dynamically.
	day := time.Unix(1700000000, 0).Local()

	t.Run("this_day_in_history", func(t *testing.T) {
		var out ThisDayOut
		callTool(t, ctx, cs, "this_day_in_history",
			map[string]any{"date": day.Format("01-02")}, &out)
		if len(out.Years) != 1 || out.Years[0].Year != day.Year() {
			t.Fatalf("years = %+v, want exactly %d", out.Years, day.Year())
		}
		if out.RecordHighF == nil || !almost(*out.RecordHighF, 68) { // 20°C
			t.Errorf("record high = %v, want 68", out.RecordHighF)
		}
		if out.RecordHighYear != day.Year() {
			t.Errorf("record high year = %d, want %d", out.RecordHighYear, day.Year())
		}
	})

	t.Run("period_summary", func(t *testing.T) {
		var out PeriodSummaryOut
		callTool(t, ctx, cs, "period_summary", map[string]any{"period": "year"}, &out)
		if out.Period != "year" {
			t.Errorf("period = %q, want year", out.Period)
		}
		var obs int64
		for _, p := range out.Periods {
			obs += p.Obs
		}
		if obs != 2 {
			t.Errorf("summed obs = %d, want 2", obs)
		}

		// Malformed period must surface as a tool error, not a silent default.
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{
			Name: "period_summary", Arguments: map[string]any{"period": "decade"}})
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		if !res.IsError {
			t.Error("period=decade should be a tool error")
		}
	})

	t.Run("wind_rose", func(t *testing.T) {
		var out WindRoseOut
		callTool(t, ctx, cs, "wind_rose", nil, &out)
		if len(out.Sectors) != 16 {
			t.Fatalf("sectors = %d, want 16", len(out.Sectors))
		}
		// The fixture has no wind data at all: everything zero, no crash.
		if out.Obs != 0 {
			t.Errorf("obs = %d, want 0 (fixture has no wind columns)", out.Obs)
		}
	})

	t.Run("climate_normals", func(t *testing.T) {
		var out ClimateNormalsOut
		callTool(t, ctx, cs, "climate_normals", nil, &out)
		// The two fixture rows are one day apart in the same month, so the normal
		// table has exactly one month backed by one year.
		if len(out.Months) != 1 || out.Months[0].Years != 1 {
			t.Fatalf("months = %+v, want one month over one year", out.Months)
		}
	})

	t.Run("rain_stats", func(t *testing.T) {
		var out RainStatsOut
		callTool(t, ctx, cs, "rain_stats", nil, &out)
		// Fixture: day one has 1.0 mm (rainy), day two 0.0 mm (dry).
		if out.DaysObserved != 2 || out.RainyDays != 1 {
			t.Errorf("observed/rainy = %d/%d, want 2/1", out.DaysObserved, out.RainyDays)
		}
		if out.WettestDayIn == nil || !almost(*out.WettestDayIn, 1.0/25.4) {
			t.Errorf("wettest = %v in, want %v", out.WettestDayIn, 1.0/25.4)
		}
	})

	t.Run("pressure_trend", func(t *testing.T) {
		// The fixture rows carry no pressure column, so the tool reports "not
		// enough history" rather than a spurious trend.
		var out PressureTrendOut
		callTool(t, ctx, cs, "pressure_trend", nil, &out)
		if out.Category != "" {
			t.Errorf("category = %q, want empty (fixture has no pressure)", out.Category)
		}
		if out.Note == "" {
			t.Error("expected a note explaining the missing trend")
		}
	})

	t.Run("climate_indices", func(t *testing.T) {
		var out ClimateIndicesOut
		callTool(t, ctx, cs, "climate_indices", map[string]any{"yearly": true}, &out)
		// Fixture days are 68°F and 77°F: no frost/ice, one summer day (77°F),
		// no hot days, two tropical nights (both lows >= 68°F).
		if out.Total.Days != 2 {
			t.Fatalf("days = %d, want 2", out.Total.Days)
		}
		if out.Total.FrostDays != 0 || out.Total.SummerDays != 1 || out.Total.TropicalNights != 2 {
			t.Errorf("total = %+v, want frost 0, summer 1, tropical 2", out.Total)
		}
		if out.FrostMaxF != 32 || out.TropicalNightMinF != 68 {
			t.Errorf("thresholds = frost %v tropical %v, want 32 and 68", out.FrostMaxF, out.TropicalNightMinF)
		}
		if len(out.Years) == 0 {
			t.Error("yearly=true should return a per-year breakdown")
		}
	})

	t.Run("climatology", func(t *testing.T) {
		var out ClimatologyOut
		callTool(t, ctx, cs, "climatology", nil, &out)
		// The two fixture rows are exactly 24h apart, so they land in the same
		// local hour: one hour slot with both observations.
		if len(out.Hours) != 1 || out.Hours[0].Obs != 2 {
			t.Fatalf("hours = %+v, want one slot with 2 obs", out.Hours)
		}
		// Mean of 20°C and 25°C = 22.5°C -> 72.5°F.
		if out.Hours[0].TempAvgF == nil || !almost(*out.Hours[0].TempAvgF, 72.5) {
			t.Errorf("hour temp avg = %v, want 72.5", out.Hours[0].TempAvgF)
		}
	})

	t.Run("degree_days", func(t *testing.T) {
		var out DegreeDaysOut
		callTool(t, ctx, cs, "degree_days", map[string]any{"monthly": true}, &out)
		// Two days at 68°F and 77°F means, default 65/65 bases:
		// CDD = 3 + 12 = 15; HDD = 0; GDD (base 50) = 18 + 27 = 45.
		if out.Total.Days != 2 {
			t.Fatalf("total days = %d, want 2", out.Total.Days)
		}
		if !almost(out.Total.CDD, 15) || !almost(out.Total.HDD, 0) || !almost(out.Total.GDD, 45) {
			t.Errorf("total = %+v, want CDD 15, HDD 0, GDD 45", out.Total)
		}
		if out.HeatingBaseF != 65 || out.GrowingBaseF != 50 {
			t.Errorf("bases = heating %v growing %v, want 65 and 50", out.HeatingBaseF, out.GrowingBaseF)
		}
		if len(out.Months) == 0 {
			t.Error("monthly=true should return a per-month breakdown")
		}
	})

	t.Run("comfort_stats", func(t *testing.T) {
		var out ComfortOut
		callTool(t, ctx, cs, "comfort_stats", nil, &out)
		// Fixture days are 68°F and 77°F, both in the mild band, so feels-like equals
		// the air temperature: hottest 77°F on day two, coldest 68°F on day one.
		if out.DaysObserved != 2 {
			t.Fatalf("days observed = %d, want 2", out.DaysObserved)
		}
		if out.HottestFeelsLikeF == nil || !almost(*out.HottestFeelsLikeF, 77) {
			t.Errorf("hottest feels-like = %v, want 77", out.HottestFeelsLikeF)
		}
		if out.ColdestFeelsLikeF == nil || !almost(*out.ColdestFeelsLikeF, 68) {
			t.Errorf("coldest feels-like = %v, want 68", out.ColdestFeelsLikeF)
		}
	})

	t.Run("lightning_activity", func(t *testing.T) {
		// The fixture carries no strikes, so the tool reports a clean no-lightning
		// result over the two observed days rather than erroring.
		var out LightningOut
		callTool(t, ctx, cs, "lightning_activity", nil, &out)
		if out.DaysObserved != 2 || out.TotalStrikes != 0 {
			t.Errorf("observed/strikes = %d/%d, want 2/0", out.DaysObserved, out.TotalStrikes)
		}
		if out.Note == "" {
			t.Error("expected a note explaining there was no lightning")
		}
	})

	t.Run("pressure_stats", func(t *testing.T) {
		// No pressure column in the fixture: days are observed but the mean is
		// absent, with a note, rather than a spurious 0.
		var out PressureStatsOut
		callTool(t, ctx, cs, "pressure_stats", nil, &out)
		if out.DaysObserved != 2 {
			t.Fatalf("days observed = %d, want 2", out.DaysObserved)
		}
		if out.MeanInHg != nil {
			t.Errorf("mean = %v, want nil (fixture has no pressure)", out.MeanInHg)
		}
		if out.Note == "" {
			t.Error("expected a note explaining the missing pressure")
		}
	})

	t.Run("temperature_spells", func(t *testing.T) {
		// Neither fixture day reaches the default 90°F heat or 32°F cold threshold,
		// so both streaks are zero over the two observed days.
		var out TemperatureSpellsOut
		callTool(t, ctx, cs, "temperature_spells", nil, &out)
		if out.DaysObserved != 2 {
			t.Fatalf("days observed = %d, want 2", out.DaysObserved)
		}
		if out.LongestHeatWaveDays != 0 || out.LongestColdSnapDays != 0 {
			t.Errorf("heat/cold = %d/%d, want 0/0", out.LongestHeatWaveDays, out.LongestColdSnapDays)
		}
		if out.HeatThresholdF != 90 || out.ColdThresholdF != 32 {
			t.Errorf("thresholds = %v/%v, want 90/32", out.HeatThresholdF, out.ColdThresholdF)
		}
	})

	t.Run("temperature_trend", func(t *testing.T) {
		// One month in one year: anomalies collapse to zero, so no slope is fitted.
		var out TemperatureTrendOut
		callTool(t, ctx, cs, "temperature_trend", nil, &out)
		if out.Years != 1 || out.SlopePerDecadeF != nil {
			t.Errorf("years/slope = %d/%v, want 1/nil", out.Years, out.SlopePerDecadeF)
		}
		if out.Note == "" {
			t.Error("expected a note explaining the short archive")
		}
	})

	t.Run("sensor_health", func(t *testing.T) {
		// The fixture carries temperature, humidity, and wind gust but no UV, solar,
		// or pressure; the tool reports every continuous sensor with its coverage.
		var out SensorHealthOut
		callTool(t, ctx, cs, "sensor_health", nil, &out)
		if out.Observations != 2 {
			t.Fatalf("observations = %d, want 2", out.Observations)
		}
		byName := map[string]store.SensorStatus{}
		for _, ss := range out.Sensors {
			byName[ss.Sensor] = ss
		}
		if temp := byName["temperature"]; temp.Readings != 2 || !almost(temp.CoveragePct, 100) {
			t.Errorf("temperature = %+v, want 2 readings at 100%%", temp)
		}
		if uv := byName["uv"]; uv.Readings != 0 {
			t.Errorf("uv = %+v, want 0 readings (fixture has no UV)", uv)
		}
	})

	t.Run("get_observations", func(t *testing.T) {
		var out GetObservationsOut
		callTool(t, ctx, cs, "get_observations", map[string]any{
			"start": day.Format("2006-01-02"),
			"end":   time.Unix(1700086400, 0).Local().Format("2006-01-02"),
		}, &out)
		var obs int64
		for _, p := range out.Points {
			obs += p.Obs
		}
		if obs != 2 {
			t.Errorf("summed obs = %d, want 2 (points: %+v)", obs, out.Points)
		}
		if out.BucketMinutes < 1 {
			t.Errorf("bucket_minutes = %d, want >= 1", out.BucketMinutes)
		}
	})

	t.Run("query_sql", func(t *testing.T) {
		var out QuerySQLOut
		callTool(t, ctx, cs, "query_sql",
			map[string]any{"sql": "SELECT COUNT(*) AS n FROM obs_st"}, &out)
		if out.RowCount != 1 || out.Columns[0] != "n" {
			t.Fatalf("unexpected result: %+v", out)
		}

		// A write must be rejected at the tool boundary.
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{
			Name: "query_sql", Arguments: map[string]any{"sql": "DELETE FROM obs_st"}})
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		if !res.IsError {
			t.Fatal("DELETE should be a tool error")
		}
	})
}

func TestIntegrationResourcesAndPrompts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cs := connectArchiveServer(t, ctx, makeTestArchive(t))

	res, err := cs.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	uris := map[string]bool{}
	for _, r := range res.Resources {
		uris[r.URI] = true
	}
	for _, want := range []string{schemaURI, dictionaryURI} {
		if !uris[want] {
			t.Errorf("resource %q not listed", want)
		}
	}

	schema, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: schemaURI})
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if len(schema.Contents) == 0 || !strings.Contains(schema.Contents[0].Text, "obs_st") {
		t.Error("schema resource should contain the obs_st DDL")
	}

	dict, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: dictionaryURI})
	if err != nil {
		t.Fatalf("read data dictionary: %v", err)
	}
	if len(dict.Contents) == 0 || !strings.Contains(dict.Contents[0].Text, "rain_mm") {
		t.Error("data dictionary should document rain_mm")
	}

	prompts, err := cs.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	names := map[string]bool{}
	for _, p := range prompts.Prompts {
		names[p.Name] = true
	}
	for _, want := range []string{"weather_report", "climate_review", "build_archive"} {
		if !names[want] {
			t.Errorf("prompt %q not listed", want)
		}
	}

	got, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{
		Name: "weather_report", Arguments: map[string]string{"focus": "wind"}})
	if err != nil {
		t.Fatalf("get prompt: %v", err)
	}
	if len(got.Messages) == 0 {
		t.Fatal("weather_report returned no messages")
	}
	if txt, ok := got.Messages[0].Content.(*mcp.TextContent); !ok || !strings.Contains(txt.Text, "wind") {
		t.Errorf("prompt should honor the focus argument; got %v", got.Messages[0].Content)
	}
}
