package mcpapp

// Coverage tests that exercise every remaining MCP tool over the real protocol:
//   - the analytics tools on a *populated* archive (the two-row fixture in
//     integration_test.go only reaches the empty-data note paths), and
//   - the live tools (list_stations, forecast, station_details, and the live
//     current_conditions path) against an in-process mock of the WeatherFlow API.
// Together with integration_test.go, history_integration_test.go, and
// archive_e2e_test.go, this leaves no registered tool without a call-and-assert.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "modernc.org/sqlite"
)

// richWant holds the expectations computed from the same rows makeRichArchive
// inserts, so assertions track the fixture instead of hard-coded magic numbers.
type richWant struct {
	rows          int
	days          int
	firstDay      string // local YYYY-MM-DD of the earliest row
	lastDay       string
	peakGustMph   float64
	maxSustainMph float64
	totalStrikes  int64
	peakSolarWm2  float64
	peakUV        float64
	minTempF      float64
	maxTempF      float64
}

// makeRichArchive writes an obs_st archive with every sensor column populated
// across several local days, and returns its path plus the expectations derived
// from the inserted rows. Values are deterministic so assertions can compare
// exact maxima/sums (aggregation-agnostic) rather than depending on the exact
// rollup path.
func makeRichArchive(t *testing.T) (string, richWant) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rich.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open temp db: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close temp db: %v", err)
		}
	}()
	ctx := t.Context()

	if _, err := db.ExecContext(ctx, `CREATE TABLE obs_st (
		epoch INTEGER NOT NULL, wind_lull REAL, wind_avg REAL, wind_gust REAL,
		wind_dir REAL, pressure_mb REAL, air_temp_c REAL, humidity REAL,
		illuminance_lux REAL, uv REAL, solar_wm2 REAL, rain_mm REAL,
		strike_dist_km REAL, strike_count REAL, battery_v REAL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	stmt, err := db.PrepareContext(ctx, `INSERT INTO obs_st
		(epoch, wind_lull, wind_avg, wind_gust, wind_dir, pressure_mb, air_temp_c,
		 humidity, illuminance_lux, uv, solar_wm2, rain_mm, strike_dist_km,
		 strike_count, battery_v) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		t.Fatalf("prepare insert: %v", err)
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			t.Errorf("close insert statement: %v", err)
		}
	}()

	const (
		base = int64(1704067200) // 2024-01-01 00:00:00 UTC, a round anchor
		n    = 72                // three days of hourly observations
	)
	days := map[string]bool{}
	var maxGust, maxSust, maxSolar, maxUV float64
	minC, maxC := 1e9, -1e9
	var strikes int64
	for i := range n {
		e := base + int64(i)*3600
		tempC := 5.0 + float64(i%16)    // 5..20 °C
		windAvg := 1.0 + float64(i%7)   // 1..7 m/s (sustained)
		gust := 3.0 + float64(i%10)     // 3..12 m/s
		dir := float64((i * 40) % 360)  // sweeps every sector
		press := 1005.0 + float64(i%21) // 1005..1025 mb
		uv := float64(i%9) * 1.0        // 0..8
		solar := float64(i%13) * 70.0   // 0..840 W/m²
		lux := solar * 100
		rain := 0.0
		if i%12 == 0 {
			rain = 0.6
		}
		var strike, sdist float64
		if i%18 == 0 && i > 0 {
			strike = float64(i%4 + 1)
			sdist = 6.0
		}
		if _, err := stmt.ExecContext(ctx, e, windAvg-0.5, windAvg, gust, dir, press, tempC,
			55.0, lux, uv, solar, rain, sdist, strike, 2.6); err != nil {
			t.Fatalf("insert row %d: %v", i, err)
		}

		day := time.Unix(e, 0).Local().Format("2006-01-02")
		days[day] = true
		maxGust = maxF(maxGust, gust)
		maxSust = maxF(maxSust, windAvg)
		maxSolar = maxF(maxSolar, solar)
		maxUV = maxF(maxUV, uv)
		minC, maxC = minF(minC, tempC), maxF(maxC, tempC)
		strikes += int64(strike)
	}

	w := richWant{
		rows:          n,
		days:          len(days),
		firstDay:      time.Unix(base, 0).Local().Format("2006-01-02"),
		lastDay:       time.Unix(base+int64(n-1)*3600, 0).Local().Format("2006-01-02"),
		peakGustMph:   model.MpsToMph(maxGust),
		maxSustainMph: model.MpsToMph(maxSust),
		totalStrikes:  strikes,
		peakSolarWm2:  maxSolar,
		peakUV:        maxUV,
		minTempF:      model.CToF(minC),
		maxTempF:      model.CToF(maxC),
	}
	return path, w
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// TestIntegrationRichArchiveTools drives the analytics tools over the protocol
// against a fully populated archive, asserting the real computed extremes/sums
// (the two-row fixture in the other files only covers the missing-data paths).
func TestIntegrationRichArchiveTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	path, w := makeRichArchive(t)
	cs := connectArchiveServer(t, ctx, path)

	t.Run("station_info", func(t *testing.T) {
		var out StationInfoOut
		callTool(t, ctx, cs, "station_info", nil, &out)
		if out.Observations != int64(w.rows) {
			t.Errorf("observations = %d, want %d", out.Observations, w.rows)
		}
		if out.FirstObs == "" || out.LastObs == "" {
			t.Errorf("first/last obs = %q/%q, want both set", out.FirstObs, out.LastObs)
		}
	})

	t.Run("daily_summary", func(t *testing.T) {
		var out DailySummaryOut
		callTool(t, ctx, cs, "daily_summary",
			map[string]any{"start": w.firstDay, "end": w.lastDay}, &out)
		if len(out.Days) != w.days {
			t.Fatalf("days = %d, want %d", len(out.Days), w.days)
		}
		var obs int64
		for _, d := range out.Days {
			obs += d.Obs
			if d.PeakGustMph == nil || d.TempMinF == nil || d.TempMaxF == nil {
				t.Errorf("day %s missing populated fields: %+v", d.Day, d)
			}
		}
		if obs != int64(w.rows) {
			t.Errorf("summed obs = %d, want %d", obs, w.rows)
		}
	})

	t.Run("records", func(t *testing.T) {
		var rec store.Records
		callTool(t, ctx, cs, "records", nil, &rec)
		if rec.HottestF == nil || !almost(*rec.HottestF, w.maxTempF) {
			t.Errorf("hottest_f = %v, want %v", rec.HottestF, w.maxTempF)
		}
		if rec.ColdestF == nil || !almost(*rec.ColdestF, w.minTempF) {
			t.Errorf("coldest_f = %v, want %v", rec.ColdestF, w.minTempF)
		}
	})

	t.Run("wind_stats", func(t *testing.T) {
		var out WindStatsOut
		callTool(t, ctx, cs, "wind_stats", nil, &out)
		if out.Obs != int64(w.rows) {
			t.Errorf("obs = %d, want %d (every row carries wind)", out.Obs, w.rows)
		}
		if out.PeakGustMph == nil || !almost(*out.PeakGustMph, w.peakGustMph) {
			t.Errorf("peak_gust_mph = %v, want %v", out.PeakGustMph, w.peakGustMph)
		}
		if out.MaxSustainedMph == nil || !almost(*out.MaxSustainedMph, w.maxSustainMph) {
			t.Errorf("max_sustained_mph = %v, want %v", out.MaxSustainedMph, w.maxSustainMph)
		}
		if out.AvgWindMph == nil {
			t.Error("avg_wind_mph is nil, want a value")
		}
	})

	t.Run("wind_rose", func(t *testing.T) {
		var out WindRoseOut
		callTool(t, ctx, cs, "wind_rose", nil, &out)
		if len(out.Sectors) != 16 {
			t.Fatalf("sectors = %d, want 16", len(out.Sectors))
		}
		if out.Obs != int64(w.rows) {
			t.Errorf("obs = %d, want %d", out.Obs, w.rows)
		}
		if out.CalmPct < 0 || out.CalmPct > 100 {
			t.Errorf("calm_pct = %v, want within [0,100]", out.CalmPct)
		}
	})

	t.Run("solar_stats", func(t *testing.T) {
		var out SolarOut
		callTool(t, ctx, cs, "solar_stats", nil, &out)
		if out.DaysObserved != int64(w.days) {
			t.Errorf("days_observed = %d, want %d", out.DaysObserved, w.days)
		}
		if out.PeakSolarWm2 == nil || !almost(*out.PeakSolarWm2, w.peakSolarWm2) {
			t.Errorf("peak_solar_wm2 = %v, want %v", out.PeakSolarWm2, w.peakSolarWm2)
		}
		if out.PeakUV == nil || !almost(*out.PeakUV, w.peakUV) {
			t.Errorf("peak_uv = %v, want %v", out.PeakUV, w.peakUV)
		}
	})

	t.Run("pressure_stats", func(t *testing.T) {
		var out PressureStatsOut
		callTool(t, ctx, cs, "pressure_stats", nil, &out)
		if out.MeanInHg == nil {
			t.Error("mean_inhg is nil, want a value (fixture carries pressure)")
		}
		if out.LowestInHg == nil || out.HighestInHg == nil || *out.LowestInHg > *out.HighestInHg {
			t.Errorf("lowest/highest = %v/%v, want lowest <= highest", out.LowestInHg, out.HighestInHg)
		}
	})

	t.Run("lightning_activity", func(t *testing.T) {
		var out LightningOut
		callTool(t, ctx, cs, "lightning_activity", nil, &out)
		if out.TotalStrikes != w.totalStrikes {
			t.Errorf("total_strikes = %d, want %d", out.TotalStrikes, w.totalStrikes)
		}
		if out.DaysObserved != int64(w.days) {
			t.Errorf("days_observed = %d, want %d", out.DaysObserved, w.days)
		}
	})

	t.Run("get_observations", func(t *testing.T) {
		var out GetObservationsOut
		callTool(t, ctx, cs, "get_observations",
			map[string]any{"start": w.firstDay, "end": w.lastDay}, &out)
		var obs int64
		for _, p := range out.Points {
			obs += p.Obs
		}
		if obs != int64(w.rows) {
			t.Errorf("summed obs = %d, want %d", obs, w.rows)
		}
	})

	// Error path: a malformed date must surface as a tool error, not empty output.
	t.Run("daily_summary_bad_date", func(t *testing.T) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{
			Name: "daily_summary", Arguments: map[string]any{"start": "not-a-date"}})
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		if !res.IsError {
			t.Error("start=not-a-date should be a tool error")
		}
	})
}

// ---- live tools -------------------------------------------------------------

// liveMockServer stands in for the WeatherFlow REST API: it serves the three
// endpoints the live tools hit (/stations, /observations/station/{id},
// /better_forecast) with a deterministic Tempest station.
func liveMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	station := api.Station{
		StationID: 123, Name: "Live Station",
		Latitude: 47.6, Longitude: -122.3, Timezone: "America/Los_Angeles",
		StationMeta: api.StationMeta{Elevation: 100},
		Devices:     []api.Device{{DeviceID: 456, DeviceType: "ST", SerialNumber: "ST-LIVE"}},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/stations":
			_ = json.NewEncoder(w).Encode(map[string]any{"stations": []api.Station{station}})
		case pathHasPrefix(r.URL.Path, "/observations/station/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"obs": []api.StationObs{{
				Timestamp: 1700000000, AirTemperature: new(float64(20)), RelativeHumidity: new(float64(55)),
				WindAvg: new(float64(4)), WindGust: new(float64(7)), WindDirection: new(float64(90)),
				SeaLevelPressure: new(float64(1013)), UV: new(float64(3)), SolarRadiation: new(float64(500)),
			}}})
		case r.URL.Path == "/better_forecast":
			var fc api.Forecast
			fc.CurrentConditions = api.ForecastCurrent{
				Time: 1700000000, Conditions: "Clear", Icon: "clear-day",
				AirTemperature: new(float64(21)), FeelsLike: new(float64(21)), RelativeHumidity: new(float64(50)),
			}
			fc.Forecast.Daily = []api.DailyForecast{{
				DayStartLocal: 1700000000, Conditions: "Clear", Icon: "clear-day",
				AirTempHigh: new(float64(25)), AirTempLow: new(float64(12)), PrecipProbability: new(10),
				Sunrise: 1700010000, Sunset: 1700050000,
			}}
			fc.Forecast.Hourly = []api.HourlyForecast{{
				Time: 1700000000, AirTemperature: new(float64(20)), WindAvg: new(float64(3)),
				WindDirection: new(float64(180)), WindGust: new(float64(5)), PrecipProbability: new(5),
			}}
			_ = json.NewEncoder(w).Encode(fc)
		default:
			http.NotFound(w, r)
		}
	}))
}

// connectLiveServer registers the live tools (no archive) against the mock API
// and returns an initialized in-memory client session.
func connectLiveServer(t *testing.T, ctx context.Context, token string) *mcp.ClientSession {
	t.Helper()
	apiClient, err := newAPIClient(token)
	if err != nil {
		t.Fatal(err)
	}
	live := &liveSource{client: apiClient}
	srv := mcp.NewServer(&mcp.Implementation{Name: "tempestkeep", Version: "test"}, nil)
	registerTools(srv, live, nil) // st=nil -> live-only

	clientT, serverT := mcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	closeOnCleanup(t, serverSession)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	closeOnCleanup(t, cs)
	return cs
}

func TestIntegrationLiveTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	srv := liveMockServer(t)
	t.Cleanup(srv.Close)
	t.Setenv("TEMPEST_API_BASE", srv.URL)
	t.Setenv("TEMPEST_THROTTLE_MS", "0")
	t.Setenv("TEMPEST_CACHE_TTL", "0")

	cs := connectLiveServer(t, ctx, "live-token")

	// The live tools must be advertised; the archive-only tools must be absent
	// because this server has no local archive.
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tl := range tools.Tools {
		byName[tl.Name] = tl
	}
	for _, name := range []string{"current_conditions", "list_stations", "forecast", "station_details"} {
		if byName[name] == nil {
			t.Errorf("live tool %q not registered", name)
		}
	}
	for _, name := range []string{"records", "daily_summary", "query_sql", "station_info"} {
		if byName[name] != nil {
			t.Errorf("archive tool %q should be absent without an archive", name)
		}
	}

	t.Run("current_conditions", func(t *testing.T) {
		var out ConditionsOut
		callTool(t, ctx, cs, "current_conditions", nil, &out)
		if out.Source != "live" {
			t.Errorf("source = %q, want live", out.Source)
		}
		if out.TempF == nil || !almost(*out.TempF, 68) { // 20°C
			t.Errorf("temp_f = %v, want 68", out.TempF)
		}
		if out.WindDir != "E" { // 90°
			t.Errorf("wind_dir = %q, want E", out.WindDir)
		}
	})

	t.Run("list_stations", func(t *testing.T) {
		var out StationsOut
		callTool(t, ctx, cs, "list_stations", nil, &out)
		if len(out.Stations) != 1 || out.Stations[0].StationID != 123 {
			t.Fatalf("stations = %+v, want one station id 123", out.Stations)
		}
		if len(out.Stations[0].Devices) != 1 || out.Stations[0].Devices[0].DeviceType != "ST" {
			t.Errorf("devices = %+v, want one ST device", out.Stations[0].Devices)
		}
	})

	t.Run("forecast", func(t *testing.T) {
		var out ForecastOut
		callTool(t, ctx, cs, "forecast", nil, &out)
		if out.Current == nil || out.Current.TempF == nil || !almost(*out.Current.TempF, model.CToF(21)) {
			t.Errorf("current temp_f wrong: %+v", out.Current)
		}
		if len(out.Daily) != 1 || out.Daily[0].HighF == nil || !almost(*out.Daily[0].HighF, 77) {
			t.Errorf("daily = %+v, want one day with high 77", out.Daily)
		}
		if len(out.Hourly) != 1 || out.Hourly[0].WindDir != "S" { // 180°
			t.Errorf("hourly = %+v, want one hour blowing S", out.Hourly)
		}
	})

	t.Run("station_details", func(t *testing.T) {
		var out StationDetailsOut
		callTool(t, ctx, cs, "station_details", nil, &out)
		if out.StationID != 123 || !almost(out.ElevationM, 100) {
			t.Errorf("station = %+v, want id 123 at 100m", out)
		}
		if len(out.Devices) != 1 || out.Devices[0].Serial != "ST-LIVE" {
			t.Errorf("devices = %+v, want one device ST-LIVE", out.Devices)
		}
	})

	// Error path: a station id the token can't see must be a tool error.
	t.Run("station_details_unknown", func(t *testing.T) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{
			Name: "station_details", Arguments: map[string]any{"station_id": 999}})
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		if !res.IsError {
			t.Error("station_id=999 should be a tool error")
		}
	})
}

// TestReadOnlyModeDropsWriteTools pins the --read-only contract: with a token
// and archive present but no writer (what run() builds under read-only), the
// append-only write tools are absent while the read-only archive_status stays.
func TestReadOnlyModeDropsWriteTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	srv := liveMockServer(t)
	t.Cleanup(srv.Close)
	t.Setenv("TEMPEST_API_BASE", srv.URL)

	st, err := store.Open(context.Background(), makeTestArchive(t))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	closeOnCleanup(t, st)

	apiClient, err := newAPIClient("ro-token")
	if err != nil {
		t.Fatal(err)
	}
	live := &liveSource{client: apiClient}
	server := mcp.NewServer(&mcp.Implementation{Name: "tempestkeep", Version: "test"}, nil)
	registerTools(server, live, st) // read-only: registerArchiveTools is NOT called

	clientT, serverT := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	closeOnCleanup(t, ss)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	closeOnCleanup(t, cs)

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := map[string]bool{}
	for _, tl := range tools.Tools {
		byName[tl.Name] = true
	}
	for _, name := range []string{"backfill_archive", "sync_archive"} {
		if byName[name] {
			t.Errorf("write tool %q must be dropped in read-only mode", name)
		}
	}
	if !byName["archive_status"] {
		t.Error("archive_status must remain in read-only mode (it only reads)")
	}
}
