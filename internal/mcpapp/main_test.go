package mcpapp

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-4 }

func TestOptionalBoundedInt(t *testing.T) {
	cases := []struct {
		v, def, max, want int
		wantErr           bool
	}{
		{0, 24, 240, 24, false},
		{-5, 24, 240, 0, true},
		{50, 24, 240, 50, false},
		{500, 24, 240, 0, true},
	}
	for _, tc := range cases {
		got, err := optionalBoundedInt(tc.v, tc.def, tc.max, "value")
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("optionalBoundedInt(%d,%d,%d) = %d, %v; want %d, error=%v",
				tc.v, tc.def, tc.max, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestPoliteThrottleRejectsInvalidValue(t *testing.T) {
	for _, value := range []string{"fast", "-1", "60001"} {
		t.Setenv("TEMPEST_THROTTLE_MS", value)
		if _, err := politeThrottle(); err == nil {
			t.Fatalf("politeThrottle accepted %q", value)
		}
	}
	t.Setenv("TEMPEST_THROTTLE_MS", "0")
	if got, err := politeThrottle(); err != nil || got != 0 {
		t.Fatalf("politeThrottle(0) = %v, %v", got, err)
	}
}

func TestTimezoneOffsetsDifferAcrossDST(t *testing.T) {
	losAngeles, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	// These zones share UTC-8 in January, but only Los Angeles changes clocks.
	now := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)
	if !timezoneOffsetsDiffer(losAngeles, phoenix, now) {
		t.Fatal("zones with different DST rules were treated as equivalent")
	}
	if timezoneOffsetsDiffer(losAngeles, losAngeles, now) {
		t.Fatal("the same timezone was treated as different")
	}
}

func TestFirstNonNil(t *testing.T) {
	v := 3.0
	if got := firstNonNil(nil, &v, nil); got == nil || *got != 3 {
		t.Errorf("got %v, want pointer to 3", got)
	}
	if got := firstNonNil(nil, nil); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestResolveRangeDefault(t *testing.T) {
	start, end, err := resolveRange(DailySummaryArgs{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// Default window is 7 whole calendar days ending today: start at local
	// midnight six days back, end at ~now.
	if want := localMidnight(now).AddDate(0, 0, -6).Unix(); start != want {
		t.Errorf("default start = %d, want %d (midnight 6 days back)", start, want)
	}
	if d := end - now.Unix(); d < -2 || d > 2 {
		t.Errorf("default end = %d, want ~now (%d)", end, now.Unix())
	}
}

func TestResolveRangeDaysAreWholeDays(t *testing.T) {
	// days=1 is exactly today (midnight..now), not a rolling 24h that would
	// spill part of yesterday into the window.
	start, _, err := resolveRange(DailySummaryArgs{Days: 1})
	if err != nil {
		t.Fatal(err)
	}
	if want := localMidnight(time.Now()).Unix(); start != want {
		t.Errorf("days=1 start = %d, want today's midnight %d", start, want)
	}
}

func TestResolveRangeEndOnly(t *testing.T) {
	// end without start must anchor the window to end, not silently ignore it
	// and return "the last N days from now" (the schema advertises end alone).
	start, end, err := resolveRange(DailySummaryArgs{Days: 3, End: "2023-06-10"})
	if err != nil {
		t.Fatal(err)
	}
	wantStart, err := parseLocalDate("2023-06-08") // 3 calendar days ending Jun 10
	if err != nil {
		t.Fatal(err)
	}
	if start != wantStart.Unix() {
		t.Errorf("end-only start = %d, want %d (Jun 8 midnight)", start, wantStart.Unix())
	}
	endDay, err := parseLocalDate("2023-06-10")
	if err != nil {
		t.Fatal(err)
	}
	if want := endDay.AddDate(0, 0, 1).Add(-time.Second).Unix(); end != want {
		t.Errorf("end-only end = %d, want %d (Jun 10 23:59:59)", end, want)
	}
}

func TestResolveRangeExplicit(t *testing.T) {
	start, end, err := resolveRange(DailySummaryArgs{Start: "2023-01-01", End: "2023-01-02"})
	if err != nil {
		t.Fatal(err)
	}
	// start = Jan 1 00:00 local, end = Jan 2 23:59:59 local -> two full days minus a second.
	if span := end - start; span != 2*86400-1 {
		t.Errorf("explicit span = %d, want %d", span, 2*86400-1)
	}
}

func TestResolveRangeBadDate(t *testing.T) {
	if _, _, err := resolveRange(DailySummaryArgs{Start: "not-a-date"}); err == nil {
		t.Error("expected error for malformed start date")
	}
}

func TestDateErrorsDoNotEchoOversizedInput(t *testing.T) {
	input := strings.Repeat("private", 10_000)
	_, err := parseLocalDate(input)
	if err == nil {
		t.Fatal("parseLocalDate accepted oversized input")
	}
	if strings.Contains(err.Error(), input) || len(err.Error()) > 128 {
		t.Fatalf("date error retained input: length=%d", len(err.Error()))
	}
	if _, _, err := resolveRange(DailySummaryArgs{Start: input}); err == nil || len(err.Error()) > 128 {
		t.Fatalf("resolveRange error is missing or unbounded: %v", err)
	}
}

func TestResolveRangeRejectsInvalidDayBounds(t *testing.T) {
	for _, days := range []int{-1, 367} {
		if _, _, err := resolveRange(DailySummaryArgs{Days: days}); err == nil {
			t.Fatalf("resolveRange accepted days=%d", days)
		}
	}
}

func TestLiveConditions(t *testing.T) {
	station := &api.Station{Name: "Test"}
	o := &api.StationObs{
		Timestamp:         1700000000,
		AirTemperature:    new(float64(20)),
		WindAvg:           new(float64(10)),
		WindDirection:     new(float64(90)),
		LightningLast1hr:  new(3),
		LightningLastDist: new(16.09344), // 10 miles
	}
	c := liveConditions(station, o)
	if c.Source != "live" || c.Station != "Test" {
		t.Errorf("unexpected header: %+v", c)
	}
	if c.TempF == nil || !almost(*c.TempF, 68) {
		t.Errorf("temp_f = %v, want 68", c.TempF)
	}
	if c.WindMph == nil || !almost(*c.WindMph, model.MpsToMph(10)) {
		t.Errorf("wind_mph = %v", c.WindMph)
	}
	if c.WindDir != "E" {
		t.Errorf("wind_dir = %q, want E", c.WindDir)
	}
	if c.LightningStrikes1hr == nil || *c.LightningStrikes1hr != 3 {
		t.Errorf("lightning strikes = %v, want 3", c.LightningStrikes1hr)
	}
	if c.LightningLastMi == nil || !almost(*c.LightningLastMi, 10) {
		t.Errorf("lightning distance = %v mi, want 10", c.LightningLastMi)
	}
}

func TestArchiveConditions(t *testing.T) {
	o := &model.Obs{Epoch: 1700000000, AirTempC: new(float64(20)), HumidityPct: new(float64(100))}
	c := archiveConditions(o)
	if c.Source != "archive" {
		t.Errorf("source = %q, want archive", c.Source)
	}
	if c.TempF == nil || !almost(*c.TempF, 68) {
		t.Errorf("temp_f = %v, want 68", c.TempF)
	}
	// Dew point at 100% RH ~ air temp (68°F).
	if c.DewPointF == nil || math.Abs(*c.DewPointF-68) > 1.0 {
		t.Errorf("dew_point_f = %v, want ~68", c.DewPointF)
	}
	// 68°F is the mild band: feels-like equals the air temperature.
	if c.FeelsLikeF == nil || !almost(*c.FeelsLikeF, 68) {
		t.Errorf("feels_like_f = %v, want 68 (mild band)", c.FeelsLikeF)
	}
}

func TestFillPressureTrend(t *testing.T) {
	// Seed an archive with pressure falling 6 mb over 3 hours, then confirm
	// current_conditions picks up the "falling rapidly" tendency from it.
	path := filepath.Join(t.TempDir(), "pt.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE obs_st (
		epoch INTEGER NOT NULL, wind_lull REAL, wind_avg REAL, wind_gust REAL,
		wind_dir REAL, pressure_mb REAL, air_temp_c REAL, humidity REAL,
		illuminance_lux REAL, uv REAL, solar_wm2 REAL, rain_mm REAL,
		strike_dist_km REAL, strike_count REAL, battery_v REAL)`); err != nil {
		t.Fatal(err)
	}
	base := int64(1700000000)
	for _, r := range []struct {
		epoch int64
		mb    float64
	}{{base, 1018}, {base + 3*3600, 1012}} {
		if _, err := db.ExecContext(t.Context(), `INSERT INTO obs_st (epoch, pressure_mb) VALUES (?,?)`, r.epoch, r.mb); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, st)

	var c ConditionsOut
	fillPressureTrend(context.Background(), st, &c)
	if c.PressureTrend != "falling rapidly" {
		t.Errorf("pressure_trend = %q, want falling rapidly", c.PressureTrend)
	}
	if c.PressureTrend3hInHg == nil || *c.PressureTrend3hInHg >= 0 {
		t.Errorf("pressure_trend_3h_inhg = %v, want a negative change", c.PressureTrend3hInHg)
	}
}

func TestArchiveConditionsFeelsLike(t *testing.T) {
	// Hot and humid: 35°C (95°F) at 60% RH derives a heat index above the air
	// temperature.
	hot := archiveConditions(&model.Obs{Epoch: 1700000000, AirTempC: new(float64(35)), HumidityPct: new(float64(60))})
	if hot.FeelsLikeF == nil || *hot.FeelsLikeF <= 95 {
		t.Errorf("hot feels_like = %v, want above 95", hot.FeelsLikeF)
	}

	// Cold and windy: -5°C (23°F) with a 10 m/s wind derives a wind chill below
	// the air temperature.
	cold := archiveConditions(&model.Obs{Epoch: 1700000000, AirTempC: new(float64(-5)), WindAvgMps: new(float64(10))})
	if cold.FeelsLikeF == nil || *cold.FeelsLikeF >= 23 {
		t.Errorf("cold feels_like = %v, want below 23", cold.FeelsLikeF)
	}

	// Hot but no humidity reading: no heat index can be derived, so feels-like is
	// omitted rather than fabricated from a zero humidity.
	noRH := archiveConditions(&model.Obs{Epoch: 1700000000, AirTempC: new(float64(35))})
	if noRH.FeelsLikeF != nil {
		t.Errorf("feels_like = %v, want nil (no humidity to build a heat index)", noRH.FeelsLikeF)
	}
}

func TestBuildForecast(t *testing.T) {
	var f api.Forecast
	f.CurrentConditions = api.ForecastCurrent{Time: 1700000000, Conditions: "Clear", AirTemperature: new(float64(20))}
	f.Forecast.Daily = []api.DailyForecast{
		{DayStartLocal: 1700000000, AirTempHigh: new(float64(25)), AirTempLow: new(float64(10)), Sunrise: 1700010000},
		{DayStartLocal: 1700086400, AirTempHigh: new(float64(24))},
	}
	f.Forecast.Hourly = []api.HourlyForecast{
		{Time: 1700000000, AirTemperature: new(float64(18)), WindAvg: new(float64(2)), WindDirection: new(float64(180))},
		{Time: 1700003600, AirTemperature: new(float64(19))},
		{Time: 1700007200, AirTemperature: new(float64(20))},
	}

	out := buildForecast(&api.Station{Name: "T"}, &f, 2, 1)
	if len(out.Daily) != 1 {
		t.Errorf("daily len = %d, want 1 (limited)", len(out.Daily))
	}
	if len(out.Hourly) != 2 {
		t.Errorf("hourly len = %d, want 2 (limited)", len(out.Hourly))
	}
	if out.Current == nil || out.Current.TempF == nil || !almost(*out.Current.TempF, 68) {
		t.Errorf("current temp_f wrong: %+v", out.Current)
	}
	if out.Daily[0].HighF == nil || !almost(*out.Daily[0].HighF, 77) {
		t.Errorf("daily high_f = %v, want 77", out.Daily[0].HighF)
	}
	if out.Daily[0].Sunrise == "" {
		t.Error("expected a formatted sunrise time")
	}
	if out.Hourly[0].WindDir != "S" {
		t.Errorf("hourly wind_dir = %q, want S", out.Hourly[0].WindDir)
	}
}
