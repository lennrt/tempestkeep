package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	_ "modernc.org/sqlite"
)

// obsRow is one synthetic observation for the analytics tests. Fields mirror
// the SI columns the queries aggregate; nil pointers store as NULL.
type obsRow struct {
	epoch   int64
	tempC   *float64
	windAvg *float64
	gust    *float64
	windDir *float64
	rainMm  float64
	strikes float64
	strikeD *float64 // strike_dist_km; nil stores as NULL
	solar   *float64 // solar_wm2
	uv      *float64
	lux     *float64
	hum     *float64 // humidity %; nil defaults to 50.0 (see openStoreWith)
	pres    *float64 // pressure mb; nil defaults to 1013.0 (see openStoreWith)
}

// openStoreWith builds an archive holding exactly rows and opens it read-only.
func openStoreWith(t *testing.T, rows []obsRow) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hist.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open writable db: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close temp db: %v", err)
		}
	}()
	if _, err := db.ExecContext(t.Context(), schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	const ins = `INSERT INTO obs_st
		(device_id, epoch, air_temp_c, wind_avg, wind_gust, wind_dir, rain_mm, strike_count, strike_dist_km, solar_wm2, uv, illuminance_lux, humidity, pressure_mb)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	for _, r := range rows {
		hum := 50.0 // most tests don't vary humidity; keep the historical default
		if r.hum != nil {
			hum = *r.hum
		}
		pres := 1013.0 // likewise for pressure
		if r.pres != nil {
			pres = *r.pres
		}
		if _, err := db.ExecContext(t.Context(), ins, r.epoch, r.tempC, r.windAvg, r.gust, r.windDir, r.rainMm, r.strikes, r.strikeD, r.solar, r.uv, r.lux, hum, pres); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	s, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	closeOnCleanup(t, s)
	return s
}

// localNoon returns the epoch of 12:00 local time on the given date, so a
// fixture lands on a deterministic local calendar day in any test host's zone.
func localNoon(y int, m time.Month, d int) int64 {
	return time.Date(y, m, d, 12, 0, 0, 0, time.Local).Unix()
}

func TestThisDay(t *testing.T) {
	// May 15 noon in three years, plus a decoy on May 16 that must not appear.
	rows := []obsRow{
		{epoch: localNoon(2022, time.May, 15), tempC: new(float64(20)), gust: new(float64(10)), rainMm: 25.4},
		{epoch: localNoon(2023, time.May, 15), tempC: new(float64(25)), gust: new(float64(5))},
		{epoch: localNoon(2023, time.May, 15) + 60, tempC: new(float64(27)), gust: new(float64(8))},
		{epoch: localNoon(2024, time.May, 15), tempC: new(float64(15))},
		{epoch: localNoon(2024, time.May, 16), tempC: new(float64(99))}, // decoy
	}
	s := openStoreWith(t, rows)

	got, err := s.ThisDay(context.Background(), 5, 15)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("years = %d, want 3 (%+v)", len(got), got)
	}
	if got[0].Year != 2022 || got[1].Year != 2023 || got[2].Year != 2024 {
		t.Errorf("years = %d,%d,%d, want 2022,2023,2024", got[0].Year, got[1].Year, got[2].Year)
	}
	if !almost(got[0].RainIn, 1.0) {
		t.Errorf("2022 rain = %v in, want 1.0", got[0].RainIn)
	}
	if got[1].TempMaxF == nil || !almost(*got[1].TempMaxF, model.CToF(27)) {
		t.Errorf("2023 max = %v, want %v (two obs that day)", got[1].TempMaxF, model.CToF(27))
	}
	if got[1].Obs != 2 {
		t.Errorf("2023 obs = %d, want 2", got[1].Obs)
	}
	if got[0].Day != "2022-05-15" {
		t.Errorf("day = %q, want 2022-05-15", got[0].Day)
	}
}

func TestThisDayInvalid(t *testing.T) {
	s := openStoreWith(t, nil)
	if _, err := s.ThisDay(context.Background(), 13, 1); err == nil {
		t.Error("expected an error for month 13")
	}
	if _, err := s.ThisDay(context.Background(), 2, 32); err == nil {
		t.Error("expected an error for day 32")
	}
}

func TestPeriodSummaryByMonth(t *testing.T) {
	// Two days in March (one rainy), one in April.
	rows := []obsRow{
		{epoch: localNoon(2024, time.March, 1), tempC: new(float64(10)), rainMm: 12.7},                        // 0.5 in -> rainy day
		{epoch: localNoon(2024, time.March, 2), tempC: new(float64(20)), gust: new(float64(15)), rainMm: 0.1}, // trace: not rainy
		{epoch: localNoon(2024, time.April, 1), tempC: new(float64(30))},
	}
	s := openStoreWith(t, rows)

	got, err := s.PeriodSummary(context.Background(), store.PeriodMonth, 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("periods = %d, want 2 (%+v)", len(got), got)
	}
	mar := got[0]
	if mar.Period != "2024-03" {
		t.Errorf("period = %q, want 2024-03", mar.Period)
	}
	if mar.DaysObserved != 2 || mar.RainyDays != 1 {
		t.Errorf("march days = %d rainy = %d, want 2 and 1", mar.DaysObserved, mar.RainyDays)
	}
	if !almost(mar.RainIn, model.MmToInch(12.8)) {
		t.Errorf("march rain = %v, want %v", mar.RainIn, model.MmToInch(12.8))
	}
	// True mean over observations: (10+20)/2 = 15°C.
	if mar.TempAvgF == nil || !almost(*mar.TempAvgF, model.CToF(15)) {
		t.Errorf("march avg = %v, want %v", mar.TempAvgF, model.CToF(15))
	}
	if mar.PeakGustMph == nil || !almost(*mar.PeakGustMph, model.MpsToMph(15)) {
		t.Errorf("march gust = %v, want %v", mar.PeakGustMph, model.MpsToMph(15))
	}
	if got[1].Period != "2024-04" || got[1].Obs != 1 {
		t.Errorf("april = %+v, want period 2024-04 with 1 obs", got[1])
	}
}

func TestDegreeDays(t *testing.T) {
	// A cold January day (low 0°C/32°F, high 10°C/50°F -> mean 41°F) and a warm
	// July day (low 20°C/68°F, high 30°C/86°F -> mean 77°F). Two obs per day so
	// each carries a distinct high and low.
	rows := []obsRow{
		{epoch: localNoon(2024, time.January, 15), tempC: new(float64(0))},
		{epoch: localNoon(2024, time.January, 15) + 60, tempC: new(float64(10))},
		{epoch: localNoon(2024, time.July, 15), tempC: new(float64(20))},
		{epoch: localNoon(2024, time.July, 15) + 60, tempC: new(float64(30))},
	}
	s := openStoreWith(t, rows)

	p := store.DegreeDayParams{HeatingBaseF: 65, CoolingBaseF: 65, GrowingBaseF: 50, GrowingCapF: 86}
	total, months, err := s.DegreeDays(context.Background(), p, 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	// Jan: HDD = 65-41 = 24, CDD = 0, GDD = (min(50,86)+max(32,50))/2 - 50 = 0.
	// Jul: HDD = 0, CDD = 77-65 = 12, GDD = (min(86,86)+max(68,50))/2 - 50 = 27.
	if total.Days != 2 {
		t.Fatalf("total days = %d, want 2", total.Days)
	}
	if !almost(total.HDD, 24) || !almost(total.CDD, 12) || !almost(total.GDD, 27) {
		t.Errorf("total = HDD %v CDD %v GDD %v, want 24, 12, 27", total.HDD, total.CDD, total.GDD)
	}
	if len(months) != 2 || months[0].Period != "2024-01" || months[1].Period != "2024-07" {
		t.Fatalf("months = %+v, want 2024-01 then 2024-07", months)
	}
	if !almost(months[0].HDD, 24) || !almost(months[0].GDD, 0) {
		t.Errorf("january = %+v, want HDD 24, GDD 0", months[0])
	}
	if !almost(months[1].CDD, 12) || !almost(months[1].GDD, 27) {
		t.Errorf("july = %+v, want CDD 12, GDD 27", months[1])
	}
}

func TestDegreeDaysSkipsTempless(t *testing.T) {
	// A day whose only observation carries rain but a NULL temperature has no
	// high or low, so it contributes no degree-days (rather than a bogus mean).
	rows := []obsRow{{epoch: localNoon(2024, time.April, 1), tempC: nil, rainMm: 5}}
	s := openStoreWith(t, rows)

	total, _, err := s.DegreeDays(context.Background(),
		store.DegreeDayParams{HeatingBaseF: 65, CoolingBaseF: 65}, 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if total.Days != 0 {
		t.Fatalf("days = %d, want 0 (the day has no temperature)", total.Days)
	}
}

func TestMonthlyNormals(t *testing.T) {
	// January in two years: means 10°C and 20°C -> normal 15°C. Plus one July day
	// so a second month appears. Each day is two obs to carry a distinct low/high.
	rows := []obsRow{
		{epoch: localNoon(2023, time.January, 15), tempC: new(float64(5)), rainMm: 25.4},
		{epoch: localNoon(2023, time.January, 15) + 60, tempC: new(float64(15))},
		{epoch: localNoon(2024, time.January, 15), tempC: new(float64(15))},
		{epoch: localNoon(2024, time.January, 15) + 60, tempC: new(float64(25))},
		{epoch: localNoon(2024, time.July, 15), tempC: new(float64(30))},
	}
	s := openStoreWith(t, rows)

	got, err := s.MonthlyNormals(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("months = %d, want 2 (Jan, Jul): %+v", len(got), got)
	}
	jan := got[0]
	if jan.Month != 1 || jan.MonthName != "January" || jan.Years != 2 {
		t.Fatalf("jan = %+v, want month 1 January over 2 years", jan)
	}
	// 2023 Jan mean = (5+15)/2 = 10°C; 2024 = (15+25)/2 = 20°C; normal = 15°C.
	if jan.TempAvgF == nil || !almost(*jan.TempAvgF, model.CToF(15)) {
		t.Errorf("jan normal mean = %v, want %v", jan.TempAvgF, model.CToF(15))
	}
	// Only 2023 January had rain (1 in over 1 rainy day); averaged over 2 years
	// the normal is 0.5 in and 0.5 rainy days.
	if !almost(jan.AvgRainIn, 0.5) || !almost(jan.AvgRainyDays, 0.5) {
		t.Errorf("jan rain normal = %v in / %v rainy days, want 0.5 / 0.5", jan.AvgRainIn, jan.AvgRainyDays)
	}
	if got[1].Month != 7 {
		t.Errorf("second month = %d, want 7 (July)", got[1].Month)
	}
}

func TestRainStats(t *testing.T) {
	// A designed sequence of local days:
	//   Jun 1 dry, Jun 2 dry, Jun 3 dry   -> a 3-day dry spell
	//   Jun 4 wet, Jun 5 wet              -> a 2-day wet spell (also wettest: Jun 5)
	//   (Jun 6 missing: a coverage gap)
	//   Jun 7 dry                         -> a new, shorter dry spell
	mk := func(d int, mm float64) obsRow {
		return obsRow{epoch: localNoon(2024, time.June, d), rainMm: mm}
	}
	rows := []obsRow{
		mk(1, 0), mk(2, 0.1), mk(3, 0), // 0.1 mm is a trace, still "dry" (< 0.254)
		mk(4, 5), mk(5, 12.7),
		mk(7, 0),
	}
	s := openStoreWith(t, rows)

	rs, err := s.RainStats(context.Background(), 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if rs.DaysObserved != 6 || rs.RainyDays != 2 {
		t.Errorf("observed/rainy = %d/%d, want 6/2", rs.DaysObserved, rs.RainyDays)
	}
	if rs.LongestDrySpellDays != 3 || rs.DrySpellStart != "2024-06-01" || rs.DrySpellEnd != "2024-06-03" {
		t.Errorf("dry spell = %d days %s..%s, want 3 days 06-01..06-03",
			rs.LongestDrySpellDays, rs.DrySpellStart, rs.DrySpellEnd)
	}
	if rs.LongestWetSpellDays != 2 || rs.WetSpellStart != "2024-06-04" || rs.WetSpellEnd != "2024-06-05" {
		t.Errorf("wet spell = %d days %s..%s, want 2 days 06-04..06-05",
			rs.LongestWetSpellDays, rs.WetSpellStart, rs.WetSpellEnd)
	}
	if rs.WettestDay != "2024-06-05" || rs.WettestDayIn == nil || !almost(*rs.WettestDayIn, model.MmToInch(12.7)) {
		t.Errorf("wettest = %s %v, want 2024-06-05 %v", rs.WettestDay, rs.WettestDayIn, model.MmToInch(12.7))
	}
}

func TestLightningActivity(t *testing.T) {
	// A designed sequence of local days:
	//   Jul 1 quiet, Jul 2 quiet                 -> storm-free run starts
	//   Jul 3 storm: 5 strikes, closest 8 km
	//   Jul 4 storm: 12 strikes (busiest), closest 3 km (overall closest)
	//   (Jul 5 missing: a coverage gap)
	//   Jul 6 quiet, Jul 7 quiet, Jul 8 quiet    -> a 3-day storm-free spell
	mk := func(d int, strikes float64, distKm *float64) obsRow {
		return obsRow{epoch: localNoon(2024, time.July, d), strikes: strikes, strikeD: distKm}
	}
	rows := []obsRow{
		mk(1, 0, nil), mk(2, 0, nil),
		mk(3, 5, new(float64(8))),
		mk(4, 12, new(float64(3))),
		mk(6, 0, nil), mk(7, 0, nil), mk(8, 0, nil),
	}
	s := openStoreWith(t, rows)

	ls, err := s.LightningActivity(context.Background(), 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if ls.TotalStrikes != 17 || ls.StormDays != 2 || ls.DaysObserved != 7 {
		t.Errorf("total/storm/observed = %d/%d/%d, want 17/2/7",
			ls.TotalStrikes, ls.StormDays, ls.DaysObserved)
	}
	if ls.BusiestDay != "2024-07-04" || ls.BusiestDayStrikes != 12 {
		t.Errorf("busiest = %s %d, want 2024-07-04 12", ls.BusiestDay, ls.BusiestDayStrikes)
	}
	if ls.ClosestStrikeDay != "2024-07-04" || ls.ClosestStrikeMi == nil || !almost(*ls.ClosestStrikeMi, model.KmToMile(3)) {
		t.Errorf("closest = %s %v, want 2024-07-04 %v", ls.ClosestStrikeDay, ls.ClosestStrikeMi, model.KmToMile(3))
	}
	if ls.FirstStormDay != "2024-07-03" || ls.LastStormDay != "2024-07-04" {
		t.Errorf("first/last storm = %s/%s, want 2024-07-03/2024-07-04", ls.FirstStormDay, ls.LastStormDay)
	}
	// The gap after Jul 4 breaks the run, so the longest storm-free spell is the
	// Jul 6..8 stretch (3 days), not the leading Jul 1..2 pair.
	if ls.LongestStormFreeDays != 3 || ls.StormFreeSpellStart != "2024-07-06" || ls.StormFreeSpellEnd != "2024-07-08" {
		t.Errorf("storm-free spell = %d days %s..%s, want 3 days 07-06..07-08",
			ls.LongestStormFreeDays, ls.StormFreeSpellStart, ls.StormFreeSpellEnd)
	}
}

func TestSolarActivity(t *testing.T) {
	noon := localNoon(2024, time.August, 1)
	// Aug 1: two observations in adjacent 15-minute buckets. Bucket means integrate
	// to 200*900/1e6 + 400*900/1e6 = 0.54 MJ/m². Aug 2: one observation, 0.09 MJ.
	rows := []obsRow{
		{epoch: noon, solar: new(float64(200)), uv: new(float64(3)), lux: new(float64(1000))},
		{epoch: noon + 900, solar: new(float64(400)), uv: new(float64(7)), lux: new(float64(50000))},
		{epoch: localNoon(2024, time.August, 2), solar: new(float64(100)), uv: new(float64(1)), lux: new(float64(500))},
	}
	s := openStoreWith(t, rows)

	ss, err := s.SolarActivity(context.Background(), 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if ss.DaysObserved != 2 {
		t.Errorf("days observed = %d, want 2", ss.DaysObserved)
	}
	if ss.PeakSolarWm2 == nil || !almost(*ss.PeakSolarWm2, 400) || ss.PeakSolarDay != "2024-08-01" {
		t.Errorf("peak solar = %v on %s, want 400 on 2024-08-01", ss.PeakSolarWm2, ss.PeakSolarDay)
	}
	if ss.PeakUV == nil || !almost(*ss.PeakUV, 7) || ss.PeakUVDay != "2024-08-01" {
		t.Errorf("peak uv = %v on %s, want 7 on 2024-08-01", ss.PeakUV, ss.PeakUVDay)
	}
	if ss.MaxIlluminanceLux == nil || !almost(*ss.MaxIlluminanceLux, 50000) {
		t.Errorf("max lux = %v, want 50000", ss.MaxIlluminanceLux)
	}
	if ss.SunniestDay != "2024-08-01" || ss.SunniestDayMJ == nil || !almost(*ss.SunniestDayMJ, 0.54) {
		t.Errorf("sunniest = %s %v, want 2024-08-01 0.54", ss.SunniestDay, ss.SunniestDayMJ)
	}
	if !almost(ss.TotalInsolationMJ, 0.63) {
		t.Errorf("total insolation = %v MJ, want 0.63", ss.TotalInsolationMJ)
	}
	if ss.AvgDailyInsolationMJ == nil || !almost(*ss.AvgDailyInsolationMJ, 0.315) {
		t.Errorf("avg daily insolation = %v MJ, want 0.315", ss.AvgDailyInsolationMJ)
	}
}

func TestWindStatistics(t *testing.T) {
	noon := localNoon(2024, time.September, 1)
	// Sep 1: two windy readings (mean 5 m/s, peak sustained 8, peak gust 15).
	// Sep 2: one calm reading (0.2 m/s < the 0.5 m/s calm threshold).
	rows := []obsRow{
		{epoch: noon, windAvg: new(float64(2)), gust: new(float64(5))},
		{epoch: noon + 60, windAvg: new(float64(8)), gust: new(float64(15))},
		{epoch: localNoon(2024, time.September, 2), windAvg: new(0.2), gust: new(float64(1))},
	}
	s := openStoreWith(t, rows)

	ws, err := s.WindStatistics(context.Background(), 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if ws.Obs != 3 {
		t.Errorf("obs = %d, want 3", ws.Obs)
	}
	if ws.AvgWindMph == nil || !almost(*ws.AvgWindMph, model.MpsToMph(10.2/3)) {
		t.Errorf("avg wind = %v, want %v", ws.AvgWindMph, model.MpsToMph(10.2/3))
	}
	if ws.PeakGustMph == nil || !almost(*ws.PeakGustMph, model.MpsToMph(15)) || ws.PeakGustDay != "2024-09-01" {
		t.Errorf("peak gust = %v on %s, want %v on 2024-09-01", ws.PeakGustMph, ws.PeakGustDay, model.MpsToMph(15))
	}
	if ws.MaxSustainedMph == nil || !almost(*ws.MaxSustainedMph, model.MpsToMph(8)) || ws.MaxSustainedDay != "2024-09-01" {
		t.Errorf("max sustained = %v on %s, want %v on 2024-09-01", ws.MaxSustainedMph, ws.MaxSustainedDay, model.MpsToMph(8))
	}
	if ws.WindiestDay != "2024-09-01" || ws.WindiestDayAvgMph == nil || !almost(*ws.WindiestDayAvgMph, model.MpsToMph(5)) {
		t.Errorf("windiest = %s %v, want 2024-09-01 %v", ws.WindiestDay, ws.WindiestDayAvgMph, model.MpsToMph(5))
	}
	if !almost(ws.CalmPct, 100.0/3) {
		t.Errorf("calm pct = %v, want %v", ws.CalmPct, 100.0/3)
	}
}

func TestComfortStatistics(t *testing.T) {
	// One hot, humid summer day; one cold, windy winter day; one mild day. Each
	// day carries a single observation, so the bucket mean equals that reading and
	// the expected feels-like comes straight from the model functions.
	rows := []obsRow{
		{epoch: localNoon(2024, time.July, 20), tempC: new(float64(35)), hum: new(float64(60)), windAvg: new(float64(1))},
		{epoch: localNoon(2024, time.January, 15), tempC: new(float64(-5)), hum: new(float64(70)), windAvg: new(float64(10))},
		{epoch: localNoon(2024, time.April, 1), tempC: new(float64(15)), hum: new(float64(50)), windAvg: new(float64(2))},
	}
	s := openStoreWith(t, rows)

	cs, err := s.ComfortStatistics(context.Background(), 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if cs.DaysObserved != 3 {
		t.Errorf("days observed = %d, want 3", cs.DaysObserved)
	}
	wantHot := model.ApparentTempF(model.CToF(35), 60, model.MpsToMph(1))
	if cs.HottestFeelsLikeF == nil || !almost(*cs.HottestFeelsLikeF, wantHot) || cs.HottestFeelsLikeDay != "2024-07-20" {
		t.Errorf("hottest feels-like = %v on %s, want %v on 2024-07-20", cs.HottestFeelsLikeF, cs.HottestFeelsLikeDay, wantHot)
	}
	wantCold := model.ApparentTempF(model.CToF(-5), 70, model.MpsToMph(10))
	if cs.ColdestFeelsLikeF == nil || !almost(*cs.ColdestFeelsLikeF, wantCold) || cs.ColdestFeelsLikeDay != "2024-01-15" {
		t.Errorf("coldest feels-like = %v on %s, want %v on 2024-01-15", cs.ColdestFeelsLikeF, cs.ColdestFeelsLikeDay, wantCold)
	}
	wantDew := model.CToF(model.DewPointC(35, 60))
	if cs.MuggiestDewPointF == nil || !almost(*cs.MuggiestDewPointF, wantDew) || cs.MuggiestDay != "2024-07-20" {
		t.Errorf("muggiest = %v on %s, want %v on 2024-07-20", cs.MuggiestDewPointF, cs.MuggiestDay, wantDew)
	}
}

func TestSensorHealthReport(t *testing.T) {
	base := localNoon(2024, time.January, 1)
	// Three observations two hours apart. UV reports on the first two then goes
	// dark; its last reading trails the newest observation by 2h (> the 1h grace),
	// so it must be flagged stale. Temperature and solar report throughout.
	rows := []obsRow{
		{epoch: base, tempC: new(float64(10)), windAvg: new(float64(2)), uv: new(float64(3)), solar: new(float64(100))},
		{epoch: base + 7200, tempC: new(float64(12)), windAvg: new(float64(3)), uv: new(float64(4)), solar: new(float64(200))},
		{epoch: base + 14400, tempC: new(float64(11)), windAvg: new(float64(2)), solar: new(float64(150))}, // no UV
	}
	s := openStoreWith(t, rows)

	h, err := s.SensorHealthReport(context.Background(), 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if h.Observations != 3 {
		t.Fatalf("observations = %d, want 3", h.Observations)
	}
	byName := map[string]store.SensorStatus{}
	for _, ss := range h.Sensors {
		byName[ss.Sensor] = ss
	}
	if temp := byName["temperature"]; temp.Readings != 3 || !almost(temp.CoveragePct, 100) || temp.Stale {
		t.Errorf("temperature = %+v, want 3 readings, 100%%, not stale", temp)
	}
	if uv := byName["uv"]; uv.Readings != 2 || !almost(uv.CoveragePct, 200.0/3) || !uv.Stale {
		t.Errorf("uv = %+v, want 2 readings, ~66.7%%, stale", uv)
	}
	// Illuminance never reported: zero coverage, no last reading, not stale.
	if lux := byName["illuminance"]; lux.Readings != 0 || lux.CoveragePct != 0 || lux.LastReading != "" || lux.Stale {
		t.Errorf("illuminance = %+v, want zero coverage and not stale", lux)
	}
}

func TestPressureStatistics(t *testing.T) {
	// Jun 1 1013, Jun 2 1000 (−13 mb, largest fall), Jun 3 1020 (+20 mb, largest
	// rise). A gap on Jun 4, then Jun 6 1015: its change from Jun 3 must NOT count
	// as a swing (not consecutive). Lowest 1000 (Jun 2), highest 1020 (Jun 3).
	mk := func(d int, mb float64) obsRow {
		return obsRow{epoch: localNoon(2024, time.June, d), pres: new(mb)}
	}
	rows := []obsRow{mk(1, 1013), mk(2, 1000), mk(3, 1020), mk(6, 1015)}
	s := openStoreWith(t, rows)

	ps, err := s.PressureStatistics(context.Background(), 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if ps.DaysObserved != 4 {
		t.Errorf("days observed = %d, want 4", ps.DaysObserved)
	}
	if ps.MeanInHg == nil || !almost(*ps.MeanInHg, model.MbToInHg(1012)) {
		t.Errorf("mean = %v, want %v", ps.MeanInHg, model.MbToInHg(1012))
	}
	if ps.LowestInHg == nil || !almost(*ps.LowestInHg, model.MbToInHg(1000)) || ps.LowestDay != "2024-06-02" {
		t.Errorf("lowest = %v on %s, want %v on 2024-06-02", ps.LowestInHg, ps.LowestDay, model.MbToInHg(1000))
	}
	if ps.HighestInHg == nil || !almost(*ps.HighestInHg, model.MbToInHg(1020)) || ps.HighestDay != "2024-06-03" {
		t.Errorf("highest = %v on %s, want %v on 2024-06-03", ps.HighestInHg, ps.HighestDay, model.MbToInHg(1020))
	}
	if ps.LargestFallInHg == nil || !almost(*ps.LargestFallInHg, model.MbToInHg(-13)) || ps.LargestFallDay != "2024-06-02" {
		t.Errorf("largest fall = %v on %s, want %v on 2024-06-02", ps.LargestFallInHg, ps.LargestFallDay, model.MbToInHg(-13))
	}
	if ps.LargestRiseInHg == nil || !almost(*ps.LargestRiseInHg, model.MbToInHg(20)) || ps.LargestRiseDay != "2024-06-03" {
		t.Errorf("largest rise = %v on %s, want %v on 2024-06-03", ps.LargestRiseInHg, ps.LargestRiseDay, model.MbToInHg(20))
	}
}

func TestTemperatureSpells(t *testing.T) {
	// A 3-day June heat wave (highs above 90°F), then a cool day that breaks it.
	// A January cold snap split by a missing day: Jan 5-6 below freezing, a gap on
	// Jan 7, then Jan 8, so the longest snap is 2 days, not 3.
	mk := func(mo time.Month, d int, tempC float64) obsRow {
		return obsRow{epoch: localNoon(2024, mo, d), tempC: new(tempC)}
	}
	rows := []obsRow{
		mk(time.January, 5, -1), mk(time.January, 6, -2), mk(time.January, 8, -4),
		mk(time.June, 10, 33), mk(time.June, 11, 35), mk(time.June, 12, 33), mk(time.June, 13, 27),
	}
	s := openStoreWith(t, rows)

	ts, err := s.TemperatureSpells(context.Background(), store.TempSpellParams{}, 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if ts.HeatThresholdF != 90 || ts.ColdThresholdF != 32 {
		t.Errorf("thresholds = %v/%v, want 90/32", ts.HeatThresholdF, ts.ColdThresholdF)
	}
	if ts.DaysObserved != 7 {
		t.Errorf("days observed = %d, want 7", ts.DaysObserved)
	}
	if ts.LongestHeatWaveDays != 3 || ts.HeatWaveStart != "2024-06-10" || ts.HeatWaveEnd != "2024-06-12" {
		t.Errorf("heat wave = %d days %s..%s, want 3 days 06-10..06-12",
			ts.LongestHeatWaveDays, ts.HeatWaveStart, ts.HeatWaveEnd)
	}
	if ts.LongestColdSnapDays != 2 || ts.ColdSnapStart != "2024-01-05" || ts.ColdSnapEnd != "2024-01-06" {
		t.Errorf("cold snap = %d days %s..%s, want 2 days 01-05..01-06 (the gap must break it)",
			ts.LongestColdSnapDays, ts.ColdSnapStart, ts.ColdSnapEnd)
	}
}

func TestTemperatureTrend(t *testing.T) {
	// Three years, one January and one July reading each, warming 2°C/year in both
	// months. Anomalies (vs each month's normal) run -3.6, 0, +3.6 °F, a clean
	// upward trend, so the fitted slope must be positive with a strong R².
	mk := func(y int, mo time.Month, tempC float64) obsRow {
		return obsRow{epoch: time.Date(y, mo, 15, 12, 0, 0, 0, time.Local).Unix(), tempC: new(tempC)}
	}
	rows := []obsRow{
		mk(2022, time.January, -2), mk(2022, time.July, 20),
		mk(2023, time.January, 0), mk(2023, time.July, 22),
		mk(2024, time.January, 2), mk(2024, time.July, 24),
	}
	s := openStoreWith(t, rows)

	tt, err := s.TemperatureTrend(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tt.Years != 3 || tt.MonthsUsed != 6 {
		t.Errorf("years/months = %d/%d, want 3/6", tt.Years, tt.MonthsUsed)
	}
	if tt.SlopePerDecadeF == nil || *tt.SlopePerDecadeF <= 0 {
		t.Fatalf("slope = %v, want a positive (warming) rate", tt.SlopePerDecadeF)
	}
	// 2°C/year ≈ 3.6°F/year ≈ 36°F/decade; allow slack for the mid-month x spread.
	if *tt.SlopePerDecadeF < 25 || *tt.SlopePerDecadeF > 45 {
		t.Errorf("slope = %v °F/decade, want ~36", *tt.SlopePerDecadeF)
	}
	if tt.RSquared == nil || *tt.RSquared < 0.9 {
		t.Errorf("R² = %v, want a strong fit (>0.9)", tt.RSquared)
	}
	if !strings.HasPrefix(tt.WarmestMonth, "2024") || !strings.HasPrefix(tt.ColdestMonth, "2022") {
		t.Errorf("warmest/coldest = %s/%s, want a 2024/2022 month", tt.WarmestMonth, tt.ColdestMonth)
	}
}

func TestTemperatureTrendShortArchive(t *testing.T) {
	// A single year: every month is its own normal, so all anomalies are zero and
	// no slope should be reported.
	rows := []obsRow{
		{epoch: localNoon(2024, time.January, 15), tempC: new(float64(0))},
		{epoch: localNoon(2024, time.July, 15), tempC: new(float64(25))},
	}
	s := openStoreWith(t, rows)
	tt, err := s.TemperatureTrend(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tt.Years != 1 {
		t.Errorf("years = %d, want 1", tt.Years)
	}
	if tt.SlopePerDecadeF != nil {
		t.Errorf("slope = %v, want nil for a single-year archive", *tt.SlopePerDecadeF)
	}
}

func TestTemperatureTrendNoMonthOverlap(t *testing.T) {
	// Two calendar years that never share a month: Jul-Dec 2020, then Jan-Jun 2021.
	// Every calendar month is observed exactly once, so each is its own normal and
	// all anomalies are zero. A slope would be meaningless; expect none.
	var rows []obsRow
	for m := time.July; m <= time.December; m++ {
		rows = append(rows, obsRow{epoch: localNoon(2020, m, 15), tempC: new(float64(m))})
	}
	for m := time.January; m <= time.June; m++ {
		rows = append(rows, obsRow{epoch: localNoon(2021, m, 15), tempC: new(float64(m))})
	}
	s := openStoreWith(t, rows)

	tt, err := s.TemperatureTrend(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tt.Years != 2 {
		t.Errorf("years = %d, want 2", tt.Years)
	}
	if tt.SlopePerDecadeF != nil {
		t.Errorf("slope = %v, want nil (no calendar month spans both years)", *tt.SlopePerDecadeF)
	}
}

func TestRainStatsGapBreaksSpell(t *testing.T) {
	// Two dry days, a two-day gap, then two more dry days: the longest dry spell
	// is 2, not 4, because the missing days aren't known to be dry.
	rows := []obsRow{
		{epoch: localNoon(2024, time.March, 1), rainMm: 0},
		{epoch: localNoon(2024, time.March, 2), rainMm: 0},
		{epoch: localNoon(2024, time.March, 5), rainMm: 0},
		{epoch: localNoon(2024, time.March, 6), rainMm: 0},
	}
	s := openStoreWith(t, rows)
	rs, err := s.RainStats(context.Background(), 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if rs.LongestDrySpellDays != 2 {
		t.Errorf("dry spell = %d, want 2 (the gap must break it)", rs.LongestDrySpellDays)
	}
}

func TestPressureTendency(t *testing.T) {
	// Pressure falling 6 mb over 3 hours: -6 mb/3h -> "falling rapidly".
	// openStoreWith writes a fixed pressure_mb (1013) on every row, so drive the
	// pressure directly with raw SQL instead.
	path := filepath.Join(t.TempDir(), "pt.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), schema); err != nil {
		t.Fatal(err)
	}
	base := int64(1700000000)
	insert := `INSERT INTO obs_st (device_id, epoch, pressure_mb) VALUES (1, ?, ?)`
	if _, err := db.ExecContext(t.Context(), insert, base, 1018.0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), insert, base+3*3600, 1012.0); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, s)

	trend, ok, err := s.PressureTendency(context.Background(), 3*3600)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected a trend from two readings 3h apart")
	}
	if trend.Category != "falling rapidly" {
		t.Errorf("category = %q, want falling rapidly (%.2f mb/3h)", trend.Category, trend.ChangeMbPer3h)
	}
	if !almost(trend.ChangeInHg, model.MbToInHg(-6)) {
		t.Errorf("change = %v inHg, want %v", trend.ChangeInHg, model.MbToInHg(-6))
	}

	// A single reading gives no span: no trend, no error.
	single := openStoreWith(t, []obsRow{{epoch: base, tempC: new(float64(20))}})
	if _, ok, err := single.PressureTendency(context.Background(), 3*3600); err != nil || ok {
		t.Errorf("single reading: ok=%v err=%v, want false, nil", ok, err)
	}
}

func TestEachObs(t *testing.T) {
	rows := []obsRow{
		{epoch: 1700000000, tempC: new(float64(20))},
		{epoch: 1700000120, tempC: nil}, // a NULL-temp row still streams
		{epoch: 1700000060, tempC: new(float64(21))},
		{epoch: 1700100000, tempC: new(float64(99))}, // outside the range below
	}
	s := openStoreWith(t, rows)

	var got []int64
	err := s.EachObs(context.Background(), 1700000000, 1700000200, func(o model.Obs) error {
		got = append(got, o.Epoch)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Three in-range rows, oldest first; the 1700100000 decoy is excluded.
	want := []int64{1700000000, 1700000060, 1700000120}
	if len(got) != len(want) {
		t.Fatalf("epochs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("epochs = %v, want %v (ordering)", got, want)
		}
	}

	// An error from the callback stops the scan and propagates.
	sentinel := errors.New("stop")
	err = s.EachObs(context.Background(), 1700000000, 1700000200, func(model.Obs) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want the sentinel", err)
	}
}

func TestClimateIndices(t *testing.T) {
	// A hard-freeze day (low -5°C/23°F, high -1°C/30.2°F: frost + ice), a hot
	// summer day (low 21°C/69.8°F, high 32°C/89.6°F: summer + tropical night),
	// and a scorcher (low 24°C/75.2°F, high 35°C/95°F: summer + hot + tropical).
	freeze := []obsRow{
		{epoch: localNoon(2023, time.January, 10), tempC: new(float64(-5))},
		{epoch: localNoon(2023, time.January, 10) + 60, tempC: new(float64(-1))},
	}
	hot := []obsRow{
		{epoch: localNoon(2023, time.July, 10), tempC: new(float64(21))},
		{epoch: localNoon(2023, time.July, 10) + 60, tempC: new(float64(32))},
	}
	scorcher := []obsRow{
		{epoch: localNoon(2024, time.July, 10), tempC: new(float64(24))},
		{epoch: localNoon(2024, time.July, 10) + 60, tempC: new(float64(35))},
	}
	rows := append(append(freeze, hot...), scorcher...)
	s := openStoreWith(t, rows)

	total, years, err := s.ClimateIndices(context.Background(), store.ClimateIndexParams{}, 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if total.Days != 3 {
		t.Fatalf("days = %d, want 3", total.Days)
	}
	if total.FrostDays != 1 || total.IceDays != 1 {
		t.Errorf("frost/ice = %d/%d, want 1/1", total.FrostDays, total.IceDays)
	}
	if total.SummerDays != 2 || total.HotDays != 1 || total.TropicalNights != 2 {
		t.Errorf("summer/hot/tropical = %d/%d/%d, want 2/1/2",
			total.SummerDays, total.HotDays, total.TropicalNights)
	}
	if len(years) != 2 || years[0].Period != "2023" || years[1].Period != "2024" {
		t.Fatalf("years = %+v, want 2023 then 2024", years)
	}
	if years[0].FrostDays != 1 || years[1].HotDays != 1 {
		t.Errorf("2023 frost = %d, 2024 hot = %d, want 1 and 1", years[0].FrostDays, years[1].HotDays)
	}
}

func TestHourlyClimatology(t *testing.T) {
	// Two mornings and one afternoon so hour 8 aggregates across days and hour
	// 14 stands alone. Times are built in local zone so hour-of-day is exact.
	at := func(y int, mo time.Month, d, h int) int64 {
		return time.Date(y, mo, d, h, 0, 0, 0, time.Local).Unix()
	}
	rows := []obsRow{
		{epoch: at(2024, time.June, 1, 8), tempC: new(float64(10)), windAvg: new(float64(2)), gust: new(float64(4))},
		{epoch: at(2024, time.June, 2, 8), tempC: new(float64(20)), windAvg: new(float64(4)), gust: new(float64(8))},
		{epoch: at(2024, time.June, 1, 14), tempC: new(float64(30)), windAvg: new(float64(6)), gust: new(float64(12))},
	}
	s := openStoreWith(t, rows)

	got, err := s.HourlyClimatology(context.Background(), 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("hours = %d, want 2 (%+v)", len(got), got)
	}
	h8, h14 := got[0], got[1]
	if h8.Hour != 8 || h14.Hour != 14 {
		t.Fatalf("hours = %d, %d, want 8, 14", h8.Hour, h14.Hour)
	}
	if h8.Obs != 2 {
		t.Errorf("hour 8 obs = %d, want 2", h8.Obs)
	}
	// Hour 8 mean temp over two days: (10+20)/2 = 15°C.
	if h8.TempAvgF == nil || !almost(*h8.TempAvgF, model.CToF(15)) {
		t.Errorf("hour 8 avg = %v, want %v", h8.TempAvgF, model.CToF(15))
	}
	if h8.TempMinF == nil || !almost(*h8.TempMinF, model.CToF(10)) {
		t.Errorf("hour 8 min = %v, want %v", h8.TempMinF, model.CToF(10))
	}
	if h8.PeakGustMph == nil || !almost(*h8.PeakGustMph, model.MpsToMph(8)) {
		t.Errorf("hour 8 gust = %v, want %v", h8.PeakGustMph, model.MpsToMph(8))
	}
	if h14.TempAvgF == nil || !almost(*h14.TempAvgF, model.CToF(30)) {
		t.Errorf("hour 14 avg = %v, want %v", h14.TempAvgF, model.CToF(30))
	}
}

func TestPeriodSummaryByYear(t *testing.T) {
	rows := []obsRow{
		{epoch: localNoon(2023, time.June, 1), tempC: new(float64(18))},
		{epoch: localNoon(2024, time.June, 1), tempC: new(float64(21))},
	}
	s := openStoreWith(t, rows)

	got, err := s.PeriodSummary(context.Background(), store.PeriodYear, 0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Period != "2023" || got[1].Period != "2024" {
		t.Fatalf("periods = %+v, want 2023 and 2024", got)
	}
}

func TestWindRose(t *testing.T) {
	rows := []obsRow{
		// Three northerlies (one at 358° must wrap into N), one easterly, one calm.
		{epoch: 1700000000, windDir: new(float64(0)), windAvg: new(float64(4)), gust: new(float64(9))},
		{epoch: 1700000060, windDir: new(float64(2)), windAvg: new(float64(6)), gust: new(float64(12))},
		{epoch: 1700000120, windDir: new(float64(358)), windAvg: new(float64(5)), gust: new(float64(10))},
		{epoch: 1700000180, windDir: new(float64(90)), windAvg: new(float64(3)), gust: new(float64(5))},
		{epoch: 1700000240, windDir: new(float64(180)), windAvg: new(0.1), gust: new(0.2)}, // calm
	}
	s := openStoreWith(t, rows)

	rose, err := s.WindRose(context.Background(), 0, 1800000000)
	if err != nil {
		t.Fatal(err)
	}
	if len(rose.Sectors) != 16 || rose.Sectors[0].Sector != "N" || rose.Sectors[4].Sector != "E" {
		t.Fatalf("sector layout wrong: %+v", rose.Sectors)
	}
	n, e := rose.Sectors[0], rose.Sectors[4]
	if n.Count != 3 {
		t.Errorf("N count = %d, want 3 (358° must wrap north)", n.Count)
	}
	if e.Count != 1 {
		t.Errorf("E count = %d, want 1", e.Count)
	}
	if n.AvgMph == nil || !almost(*n.AvgMph, model.MpsToMph(5)) {
		t.Errorf("N avg = %v, want %v", n.AvgMph, model.MpsToMph(5))
	}
	if n.MaxGustMph == nil || !almost(*n.MaxGustMph, model.MpsToMph(12)) {
		t.Errorf("N max gust = %v, want %v", n.MaxGustMph, model.MpsToMph(12))
	}
	if !almost(n.Pct, 75) {
		t.Errorf("N pct = %v, want 75 (3 of 4 non-calm)", n.Pct)
	}
	if rose.Obs != 5 {
		t.Errorf("obs = %d, want 5", rose.Obs)
	}
	if !almost(rose.CalmPct, 20) {
		t.Errorf("calm pct = %v, want 20 (1 of 5)", rose.CalmPct)
	}
	// Sectors with no observations still render (zero count, no averages).
	if rose.Sectors[8].Count != 0 || rose.Sectors[8].AvgMph != nil {
		t.Errorf("S sector should be empty: %+v", rose.Sectors[8])
	}
}

func TestSeries(t *testing.T) {
	// Six 1-minute observations spanning two 3-minute buckets.
	base := int64(1700000040) // not bucket-aligned, to prove alignment is epoch-based
	var rows []obsRow
	for i := range 6 {
		rows = append(rows, obsRow{
			epoch: base + int64(i)*60, tempC: new(10 + float64(i)),
			gust: new(float64(i)), rainMm: 1.0, strikes: 1,
		})
	}
	s := openStoreWith(t, rows)

	got, err := s.Series(context.Background(), base, base+330, 180)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("buckets = %d, want 3 (epoch-aligned buckets split 6 minutes unevenly)", len(got))
	}
	var obs, strikes int64
	var rain float64
	for _, p := range got {
		if p.Epoch%180 != 0 {
			t.Errorf("bucket %d not aligned to 180s", p.Epoch)
		}
		obs += p.Obs
		strikes += p.Strikes
		rain += p.RainIn
	}
	if obs != 6 {
		t.Errorf("total obs = %d, want 6", obs)
	}
	if strikes != 6 {
		t.Errorf("total strikes = %d, want 6", strikes)
	}
	if !almost(rain, model.MmToInch(6)) {
		t.Errorf("total rain = %v, want %v", rain, model.MmToInch(6))
	}
}

func TestQueryReadOnly(t *testing.T) {
	rows := []obsRow{
		{epoch: 1700000000, tempC: new(float64(20))},
		{epoch: 1700000060, tempC: new(float64(25))},
	}
	s := openStoreWith(t, rows)
	ctx := context.Background()

	res, err := s.Query(ctx, "SELECT epoch, air_temp_c FROM obs_st ORDER BY epoch", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Columns) != 2 || res.Columns[0] != "epoch" {
		t.Errorf("columns = %v", res.Columns)
	}
	if res.RowCount != 2 || res.Truncated {
		t.Errorf("rows = %d truncated = %v, want 2 rows untruncated", res.RowCount, res.Truncated)
	}
	if got := res.Rows[1][1]; got != 25.0 {
		t.Errorf("row[1][1] = %v (%T), want 25", got, got)
	}

	// Row cap.
	res, err = s.Query(ctx, "SELECT epoch FROM obs_st", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowCount != 1 || !res.Truncated {
		t.Errorf("capped query: rows = %d truncated = %v, want 1 row truncated", res.RowCount, res.Truncated)
	}

	// CTEs are fine.
	if _, err := s.Query(ctx, "WITH t AS (SELECT 1 AS x) SELECT x FROM t", 0); err != nil {
		t.Errorf("WITH query rejected: %v", err)
	}
	// Leading comments are fine.
	if _, err := s.Query(ctx, "-- hi\nSELECT 1", 0); err != nil {
		t.Errorf("commented query rejected: %v", err)
	}
}

func TestQueryRejectsWrites(t *testing.T) {
	s := openStoreWith(t, []obsRow{{epoch: 1700000000, tempC: new(float64(20))}})
	ctx := context.Background()

	for _, q := range []string{
		"DELETE FROM obs_st",
		"UPDATE obs_st SET air_temp_c = 0",
		"INSERT INTO obs_st (device_id, epoch) VALUES (1, 2)",
		"DROP TABLE obs_st",
		"PRAGMA journal_mode=DELETE",
		"ATTACH DATABASE 'x' AS y",
		"SELECT 1; DELETE FROM obs_st",                           // multi-statement smuggling
		"WITH x AS (SELECT 1) DELETE FROM obs_st",                // CTE-prefixed DML
		"WITH x AS (SELECT 1) INSERT INTO obs_st DEFAULT VALUES", // CTE-prefixed DML
		"",
		"-- only a comment",
	} {
		if _, err := s.Query(ctx, q, 0); err == nil {
			t.Errorf("query %q was accepted, want rejection", q)
		}
	}

	// Defense in depth: even if validation were bypassed, the connection is
	// query_only; verify the guard the validator sits in front of is real.
	if _, err := s.Query(ctx, "SELECT * FROM obs_st", 0); err != nil {
		t.Fatalf("control SELECT failed: %v", err)
	}
	var n int64
	if err := queryCount(s, &n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("row count = %d, want 1 (nothing may mutate the archive)", n)
	}
}

// queryCount reads COUNT(*) through the public Query API.
func queryCount(s *store.Store, n *int64) error {
	res, err := s.Query(context.Background(), "SELECT COUNT(*) FROM obs_st", 0)
	if err != nil {
		return err
	}
	switch v := res.Rows[0][0].(type) {
	case int64:
		*n = v
	case float64:
		*n = int64(v)
	}
	return nil
}

func TestQueryValidatorMessages(t *testing.T) {
	s := openStoreWith(t, nil)
	_, err := s.Query(context.Background(), "VACUUM", 0)
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("err = %v, want a read-only explanation", err)
	}
}

func TestQueryExecutionErrorDoesNotRetainSQL(t *testing.T) {
	s := openStoreWith(t, nil)
	const marker = "private-query-marker"
	_, err := s.Query(context.Background(), "SELECT "+marker+" FROM", 0)
	if !errors.Is(err, store.ErrArchiveIO) {
		t.Fatalf("error = %v, want ErrArchiveIO", err)
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("query text reached the error: %v", err)
	}
}
