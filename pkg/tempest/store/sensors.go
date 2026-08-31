package store

// Sensor queries aggregate indexed epochs in SQL. Go converts 15-minute buckets
// to local days and converts display values to US units.

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

// lightningDay is one local calendar day of lightning activity, in SI units.
type lightningDay struct {
	day       time.Time // local midnight
	strikes   int64
	closestKm *float64 // nearest strike that day, nil when none
}

// LightningStats summarizes lightning in US display units. A storm day has at
// least one detected strike. A coverage gap ends a storm-free run.
type LightningStats struct {
	TotalStrikes int64 `json:"total_strikes"`
	DaysObserved int64 `json:"days_observed"`
	StormDays    int64 `json:"storm_days"`

	// ClosestStrikeMi is the smallest reported interval-average distance. It is
	// not the distance to a specific bolt.
	ClosestStrikeMi  *float64 `json:"closest_strike_mi,omitempty"`
	ClosestStrikeDay string   `json:"closest_strike_day,omitempty"`

	BusiestDay        string `json:"busiest_day,omitempty"`
	BusiestDayStrikes int64  `json:"busiest_day_strikes,omitempty"`

	FirstStormDay string `json:"first_storm_day,omitempty"`
	LastStormDay  string `json:"last_storm_day,omitempty"`

	LongestStormFreeDays int64  `json:"longest_storm_free_days"`
	StormFreeSpellStart  string `json:"storm_free_spell_start,omitempty"`
	StormFreeSpellEnd    string `json:"storm_free_spell_end,omitempty"`
}

// LightningActivity aggregates lightning over [startEpoch, endEpoch]. A
// coverage gap ends a storm-free run.
func (s *Store) LightningActivity(ctx context.Context, startEpoch, endEpoch int64) (LightningStats, error) {
	days, err := s.lightningDays(ctx, startEpoch, endEpoch)
	if err != nil {
		return LightningStats{}, err
	}
	var ls LightningStats
	var bestCloseKm *float64
	var freeLen int64
	var prev time.Time
	havePrev := false

	fmtDay := func(t time.Time) string { return t.Format("2006-01-02") }
	for _, d := range days {
		ls.DaysObserved++
		ls.TotalStrikes += d.strikes
		hadStorm := d.strikes > 0
		if hadStorm {
			ls.StormDays++
			if ls.FirstStormDay == "" {
				ls.FirstStormDay = fmtDay(d.day)
			}
			ls.LastStormDay = fmtDay(d.day)
			if d.strikes > ls.BusiestDayStrikes {
				ls.BusiestDayStrikes = d.strikes
				ls.BusiestDay = fmtDay(d.day)
			}
			if d.closestKm != nil && (bestCloseKm == nil || *d.closestKm < *bestCloseKm) {
				v := *d.closestKm
				bestCloseKm = &v
				ls.ClosestStrikeDay = fmtDay(d.day)
			}
		}

		// A storm-free spell is a run of consecutive observed days with no
		// strikes; the first day or a coverage gap restarts the count.
		if !havePrev || !d.day.Equal(prev.AddDate(0, 0, 1)) {
			freeLen = 0
		}
		if hadStorm {
			freeLen = 0
		} else {
			freeLen++
			if freeLen > ls.LongestStormFreeDays {
				ls.LongestStormFreeDays = freeLen
				ls.StormFreeSpellEnd = fmtDay(d.day)
				ls.StormFreeSpellStart = fmtDay(d.day.AddDate(0, 0, int(-(freeLen - 1))))
			}
		}
		prev, havePrev = d.day, true
	}
	if bestCloseKm != nil {
		mi := model.KmToMile(*bestCloseKm)
		ls.ClosestStrikeMi = &mi
	}
	return ls, nil
}

// solarDay is one local calendar day of solar activity, in SI units.
type solarDay struct {
	day       time.Time // local midnight
	energyMJ  float64   // integrated insolation, MJ/m²
	peakWm2   *float64
	peakUV    *float64
	peakLux   *float64
	hasEnergy bool // at least one bucket carried irradiance
}

// SolarStats summarizes the solar/UV package over a date range, in mixed units
// growers and forecasters use: irradiance in W/m², daily insolation in MJ/m²,
// the UV index unitless, and illuminance in lux. Daily insolation integrates the
// 15-minute bucket means over their span, so a day with coverage gaps reports the
// energy actually observed rather than an extrapolated full day.
type SolarStats struct {
	DaysObserved int64 `json:"days_observed"`

	PeakSolarWm2 *float64 `json:"peak_solar_wm2,omitempty"`
	PeakSolarDay string   `json:"peak_solar_day,omitempty"`

	PeakUV    *float64 `json:"peak_uv,omitempty"`
	PeakUVDay string   `json:"peak_uv_day,omitempty"`

	MaxIlluminanceLux *float64 `json:"max_illuminance_lux,omitempty"`
	MaxIlluminanceDay string   `json:"max_illuminance_day,omitempty"`

	SunniestDay   string   `json:"sunniest_day,omitempty"`
	SunniestDayMJ *float64 `json:"sunniest_day_mj,omitempty"`

	// AvgDailyInsolationMJ averages the daily insolation over days that carried at
	// least one solar reading, so a fully dark-sensor day doesn't dilute it.
	AvgDailyInsolationMJ *float64 `json:"avg_daily_insolation_mj,omitempty"`
	TotalInsolationMJ    float64  `json:"total_insolation_mj"`
}

// SolarActivity aggregates the solar/UV package over [startEpoch, endEpoch]: the
// peak irradiance, UV index, and illuminance with the days they occurred, plus
// daily insolation (MJ/m²) integrated from the bucket means, its sunniest day,
// and the average over days that carried a solar reading. It answers "how much
// sun did the garden get?" and "when was UV worst?" offline.
func (s *Store) SolarActivity(ctx context.Context, startEpoch, endEpoch int64) (SolarStats, error) {
	days, err := s.solarDays(ctx, startEpoch, endEpoch)
	if err != nil {
		return SolarStats{}, err
	}
	var ss SolarStats
	var peakSolar, peakUV, peakLux, bestEnergy *float64
	var energyDays int64
	for _, d := range days {
		ss.DaysObserved++
		if d.peakWm2 != nil && (peakSolar == nil || *d.peakWm2 > *peakSolar) {
			v := *d.peakWm2
			peakSolar = &v
			ss.PeakSolarDay = d.day.Format("2006-01-02")
		}
		if d.peakUV != nil && (peakUV == nil || *d.peakUV > *peakUV) {
			v := *d.peakUV
			peakUV = &v
			ss.PeakUVDay = d.day.Format("2006-01-02")
		}
		if d.peakLux != nil && (peakLux == nil || *d.peakLux > *peakLux) {
			v := *d.peakLux
			peakLux = &v
			ss.MaxIlluminanceDay = d.day.Format("2006-01-02")
		}
		if d.hasEnergy {
			ss.TotalInsolationMJ += d.energyMJ
			energyDays++
			if bestEnergy == nil || d.energyMJ > *bestEnergy {
				v := d.energyMJ
				bestEnergy = &v
				ss.SunniestDay = d.day.Format("2006-01-02")
			}
		}
	}
	ss.PeakSolarWm2 = peakSolar
	ss.PeakUV = peakUV
	ss.MaxIlluminanceLux = peakLux
	ss.SunniestDayMJ = bestEnergy
	if energyDays > 0 {
		avg := ss.TotalInsolationMJ / float64(energyDays)
		ss.AvgDailyInsolationMJ = &avg
	}
	return ss, nil
}

// staleGraceSeconds is how far a sensor's last reading may trail the archive's
// newest observation before it counts as stale (gone dark): one hour, generous
// enough not to flag a sensor on a normal reporting cadence.
const staleGraceSeconds = 3600

// SensorStatus is the health of one continuous sensor over a date range: how
// often it reported and when it last did. A stale sensor last reported well
// before the archive's newest observation, the signal that it has gone dark.
type SensorStatus struct {
	Sensor      string  `json:"sensor"`
	Readings    int64   `json:"readings"`
	CoveragePct float64 `json:"coverage_pct"`
	LastReading string  `json:"last_reading,omitempty"` // local time of the newest non-null value
	Stale       bool    `json:"stale,omitempty"`
}

// SensorHealth is a per-sensor diagnostic over a date range: which sensors are
// reporting, how completely, and whether any have gone dark, plus the battery
// voltage range. It answers "is my station healthy?" from the archive, the
// operational companion to the climate analytics.
type SensorHealth struct {
	Observations   int64          `json:"observations"`
	NewestObsEpoch int64          `json:"newest_obs_epoch,omitempty"`
	Sensors        []SensorStatus `json:"sensors"`
	BatteryMinV    *float64       `json:"battery_min_v,omitempty"`
	BatteryMaxV    *float64       `json:"battery_max_v,omitempty"`
}

// SensorHealthReport reports the reading count, coverage, and last-reading time
// for each continuous sensor over [startEpoch, endEpoch], flagging any that last
// reported more than an hour before the archive's newest observation as stale
// (gone dark), plus the battery voltage range. The event sensors (rain,
// lightning) are excluded: a null there usually means "nothing happened", not a
// fault. A single scan gathers every count and last-seen epoch at once.
func (s *Store) SensorHealthReport(ctx context.Context, startEpoch, endEpoch int64) (SensorHealth, error) {
	var h SensorHealth
	if err := validateEpochRange(startEpoch, endEpoch); err != nil {
		return h, err
	}
	db, err := s.database()
	if err != nil {
		return h, err
	}
	var total int64
	// One (count, lastEpoch) pair per sensor, in the SQL's column order.
	var tempN, humN, presN, windN, uvN, solarN, luxN, battN int64
	var tempE, humE, presE, windE, uvE, solarE, luxE sql.NullInt64
	var battMin, battMax sql.NullFloat64
	var newest sql.NullInt64
	if err := db.QueryRowContext(ctx, qrySensorHealth, startEpoch, endEpoch).Scan(
		&total,
		&tempN, &tempE, &humN, &humE, &presN, &presE, &windN, &windE,
		&uvN, &uvE, &solarN, &solarE, &luxN, &luxE,
		&battN, &battMin, &battMax, &newest,
	); err != nil {
		return h, archiveFailure("read sensor health", err)
	}
	h.Observations = total
	if total == 0 || !newest.Valid {
		return h, nil // empty range: no sensors to report
	}
	h.NewestObsEpoch = newest.Int64

	add := func(name string, n int64, last sql.NullInt64) {
		st := SensorStatus{
			Sensor: name, Readings: n,
			CoveragePct: 100 * float64(n) / float64(total),
		}
		if last.Valid {
			st.LastReading = localTime(last.Int64)
			st.Stale = last.Int64 < newest.Int64-staleGraceSeconds
		}
		h.Sensors = append(h.Sensors, st)
	}
	add("temperature", tempN, tempE)
	add("humidity", humN, humE)
	add("pressure", presN, presE)
	add("wind", windN, windE)
	add("uv", uvN, uvE)
	add("solar", solarN, solarE)
	add("illuminance", luxN, luxE)

	h.BatteryMinV = nf(battMin)
	h.BatteryMaxV = nf(battMax)
	return h, nil
}

// pressureDay is one local calendar day of station pressure, in millibars.
type pressureDay struct {
	day   time.Time // local midnight
	sum   float64   // Σ pressure_mb over the day's observations
	n     int64
	minMb *float64
	maxMb *float64
}

// PressureStats summarizes station (not sea-level) pressure over a date range, in
// inches of mercury. The largest day-over-day swings are the change in the daily
// mean between consecutive observed days, a proxy for storm intensity: a sharp
// fall precedes deteriorating weather, a sharp rise the clearing behind it.
type PressureStats struct {
	DaysObserved int64 `json:"days_observed"`

	MeanInHg *float64 `json:"mean_inhg,omitempty"`

	LowestInHg  *float64 `json:"lowest_inhg,omitempty"`
	LowestDay   string   `json:"lowest_day,omitempty"`
	HighestInHg *float64 `json:"highest_inhg,omitempty"`
	HighestDay  string   `json:"highest_day,omitempty"`

	LargestFallInHg *float64 `json:"largest_daily_fall_inhg,omitempty"` // biggest day-over-day drop in the daily mean
	LargestFallDay  string   `json:"largest_daily_fall_day,omitempty"`
	LargestRiseInHg *float64 `json:"largest_daily_rise_inhg,omitempty"`
	LargestRiseDay  string   `json:"largest_daily_rise_day,omitempty"`
}

// PressureStatistics aggregates station pressure over [startEpoch, endEpoch]: the
// mean, the lowest and highest readings with their days, and the largest
// day-over-day swings in the daily mean (a storm-intensity proxy). Day-over-day
// changes only compare consecutive observed days, so a coverage gap never invents
// a swing across days the archive never recorded.
func (s *Store) PressureStatistics(ctx context.Context, startEpoch, endEpoch int64) (PressureStats, error) {
	days, err := s.pressureDays(ctx, startEpoch, endEpoch)
	if err != nil {
		return PressureStats{}, err
	}
	var ps PressureStats
	var lowMb, highMb, largestFallMb, largestRiseMb *float64
	var globalSum float64
	var globalN int64
	var prevMean *float64
	var prevDay time.Time
	havePrev := false

	for _, d := range days {
		ps.DaysObserved++
		globalSum += d.sum
		globalN += d.n
		if d.minMb != nil && (lowMb == nil || *d.minMb < *lowMb) {
			v := *d.minMb
			lowMb = &v
			ps.LowestDay = d.day.Format("2006-01-02")
		}
		if d.maxMb != nil && (highMb == nil || *d.maxMb > *highMb) {
			v := *d.maxMb
			highMb = &v
			ps.HighestDay = d.day.Format("2006-01-02")
		}

		if d.n > 0 {
			mean := d.sum / float64(d.n)
			consecutive := havePrev && d.day.Equal(prevDay.AddDate(0, 0, 1))
			if consecutive && prevMean != nil {
				delta := mean - *prevMean
				if delta < 0 && (largestFallMb == nil || delta < *largestFallMb) {
					v := delta
					largestFallMb = &v
					ps.LargestFallDay = d.day.Format("2006-01-02")
				}
				if delta > 0 && (largestRiseMb == nil || delta > *largestRiseMb) {
					v := delta
					largestRiseMb = &v
					ps.LargestRiseDay = d.day.Format("2006-01-02")
				}
			}
			prevMean = &mean
			prevDay = d.day
			havePrev = true
		} else {
			// A day with no pressure reading breaks the consecutive-day chain.
			havePrev = false
		}
	}

	if globalN > 0 {
		v := model.MbToInHg(globalSum / float64(globalN))
		ps.MeanInHg = &v
	}
	if lowMb != nil {
		v := model.MbToInHg(*lowMb)
		ps.LowestInHg = &v
	}
	if highMb != nil {
		v := model.MbToInHg(*highMb)
		ps.HighestInHg = &v
	}
	if largestFallMb != nil {
		v := model.MbToInHg(*largestFallMb) // negative: a drop
		ps.LargestFallInHg = &v
	}
	if largestRiseMb != nil {
		v := model.MbToInHg(*largestRiseMb)
		ps.LargestRiseInHg = &v
	}
	return ps, nil
}

// pressureDays rolls the observations in [startEpoch, endEpoch] up to local
// calendar days, oldest first, carrying each day's pressure sum and count (for
// the daily mean) and the daily pressure extremes.
func (s *Store) pressureDays(ctx context.Context, startEpoch, endEpoch int64) (_ []pressureDay, err error) {
	rows, err := s.queryRange(ctx, startEpoch, endEpoch, qryPressureRollup, rollupBucketSeconds, startEpoch, endEpoch)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, archiveFailure("close pressure days", rows.Close())) }()

	var out []pressureDay
	for rows.Next() {
		var b, n int64
		var sum float64
		var minMb, maxMb sql.NullFloat64
		if err := rows.Scan(&b, &sum, &n, &minMb, &maxMb); err != nil {
			return nil, archiveFailure("scan pressure day", err)
		}
		t := time.Unix(b*rollupBucketSeconds, 0).Local()
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
		if len(out) == 0 || !out[len(out)-1].day.Equal(day) {
			out = append(out, pressureDay{day: day})
		}
		cur := &out[len(out)-1]
		cur.sum += sum
		cur.n += n
		cur.minMb = minPtr(cur.minMb, nf(minMb))
		cur.maxMb = maxPtr(cur.maxMb, nf(maxMb))
	}
	return out, archiveFailure("read pressure days", rows.Err())
}

// comfortDay is one local calendar day of human-comfort extremes, in °F.
type comfortDay struct {
	day          time.Time // local midnight
	hottestFeels *float64  // highest apparent temp that day
	coldestFeels *float64  // lowest apparent temp that day
	maxDewPoint  *float64  // highest dew point that day
}

// ComfortStats summarizes the human-comfort extremes over a date range, in °F:
// the hottest and coldest "feels like" (apparent) temperature and the muggiest
// day by dew point, each with the day it occurred. Apparent temperature is the
// heat index in warm air and the wind chill in cold air; dew point is the best
// single measure of how oppressive the humidity felt.
type ComfortStats struct {
	DaysObserved int64 `json:"days_observed"`

	HottestFeelsLikeF   *float64 `json:"hottest_feels_like_f,omitempty"`
	HottestFeelsLikeDay string   `json:"hottest_feels_like_day,omitempty"`

	ColdestFeelsLikeF   *float64 `json:"coldest_feels_like_f,omitempty"`
	ColdestFeelsLikeDay string   `json:"coldest_feels_like_day,omitempty"`

	MuggiestDewPointF *float64 `json:"muggiest_dew_point_f,omitempty"`
	MuggiestDay       string   `json:"muggiest_day,omitempty"`
}

// ComfortStatistics aggregates the apparent-temperature and dew-point extremes
// over [startEpoch, endEpoch]: the hottest and coldest "feels like" and the
// muggiest day. It works from 15-minute bucket means, so the temperature,
// humidity, and wind feeding each feels-like reading belong to the same quarter
// hour rather than being averaged across the whole day.
func (s *Store) ComfortStatistics(ctx context.Context, startEpoch, endEpoch int64) (ComfortStats, error) {
	days, err := s.comfortDays(ctx, startEpoch, endEpoch)
	if err != nil {
		return ComfortStats{}, err
	}
	var cs ComfortStats
	var hottest, coldest, muggiest *float64
	for _, d := range days {
		cs.DaysObserved++
		if d.hottestFeels != nil && (hottest == nil || *d.hottestFeels > *hottest) {
			v := *d.hottestFeels
			hottest = &v
			cs.HottestFeelsLikeDay = d.day.Format("2006-01-02")
		}
		if d.coldestFeels != nil && (coldest == nil || *d.coldestFeels < *coldest) {
			v := *d.coldestFeels
			coldest = &v
			cs.ColdestFeelsLikeDay = d.day.Format("2006-01-02")
		}
		if d.maxDewPoint != nil && (muggiest == nil || *d.maxDewPoint > *muggiest) {
			v := *d.maxDewPoint
			muggiest = &v
			cs.MuggiestDay = d.day.Format("2006-01-02")
		}
	}
	cs.HottestFeelsLikeF = hottest
	cs.ColdestFeelsLikeF = coldest
	cs.MuggiestDewPointF = muggiest
	return cs, nil
}

// comfortDays rolls the observations in [startEpoch, endEpoch] up to local
// calendar days, oldest first, computing each 15-minute bucket's apparent
// temperature and dew point from its contemporaneous means and keeping the daily
// extremes.
func (s *Store) comfortDays(ctx context.Context, startEpoch, endEpoch int64) (_ []comfortDay, err error) {
	rows, err := s.queryRange(ctx, startEpoch, endEpoch, qryComfortRollup, rollupBucketSeconds, startEpoch, endEpoch)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, archiveFailure("close comfort days", rows.Close())) }()

	var out []comfortDay
	for rows.Next() {
		var b, tempN int64
		var tempC, hum, wind sql.NullFloat64
		if err := rows.Scan(&b, &tempC, &hum, &wind, &tempN); err != nil {
			return nil, archiveFailure("scan comfort day", err)
		}
		if !tempC.Valid {
			continue // no temperature in this bucket: nothing to feel
		}
		t := time.Unix(b*rollupBucketSeconds, 0).Local()
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
		if len(out) == 0 || !out[len(out)-1].day.Equal(day) {
			out = append(out, comfortDay{day: day})
		}
		cur := &out[len(out)-1]

		tF := model.CToF(tempC.Float64)
		// Missing humidity or wind fall back to 0, matching current_conditions:
		// the apparent-temp formulas ignore the missing side (dry air, still air)
		// rather than refusing to report.
		var rh, windMph float64
		if hum.Valid {
			rh = hum.Float64
		}
		if wind.Valid {
			windMph = model.MpsToMph(wind.Float64)
		}
		feels := model.ApparentTempF(tF, rh, windMph)
		cur.hottestFeels = maxPtr(cur.hottestFeels, &feels)
		cur.coldestFeels = minPtr(cur.coldestFeels, &feels)

		if hum.Valid {
			if dpC := model.DewPointC(tempC.Float64, hum.Float64); !math.IsNaN(dpC) {
				dpF := model.CToF(dpC)
				cur.maxDewPoint = maxPtr(cur.maxDewPoint, &dpF)
			}
		}
	}
	return out, archiveFailure("read comfort days", rows.Err())
}

// windDay is one local calendar day of wind magnitude, in SI units.
type windDay struct {
	day          time.Time // local midnight
	windSum      float64   // Σ wind_avg over the day's observations
	windN        int64
	calm         int64 // observations below the calm threshold
	maxSustained *float64
	maxGust      *float64
}

// WindStats summarizes wind speed over a date range, in US display units, the
// magnitude companion to the wind_rose (which covers direction). The calm share
// is the fraction of wind observations below the calm threshold; the windiest day
// is the one with the highest observation-weighted average wind.
type WindStats struct {
	Obs int64 `json:"obs"` // observations carrying wind data

	AvgWindMph *float64 `json:"avg_wind_mph,omitempty"`

	MaxSustainedMph *float64 `json:"max_sustained_mph,omitempty"`
	MaxSustainedDay string   `json:"max_sustained_day,omitempty"`

	PeakGustMph *float64 `json:"peak_gust_mph,omitempty"`
	PeakGustDay string   `json:"peak_gust_day,omitempty"`

	WindiestDay       string   `json:"windiest_day,omitempty"`
	WindiestDayAvgMph *float64 `json:"windiest_day_avg_mph,omitempty"`

	CalmPct          float64 `json:"calm_pct"`
	CalmThresholdMph float64 `json:"calm_threshold_mph"`
}

// WindStatistics aggregates wind speed over [startEpoch, endEpoch]: the mean wind,
// the peak gust and peak sustained wind with the days they occurred, the windiest
// day by daily average, and the calm share. Direction lives in WindRose; this is
// the magnitude view. Days are observation-weighted so a day with more readings
// carries proportionally more of the average.
func (s *Store) WindStatistics(ctx context.Context, startEpoch, endEpoch int64) (WindStats, error) {
	ws := WindStats{CalmThresholdMph: model.MpsToMph(calmThresholdMps)}
	days, err := s.windDays(ctx, startEpoch, endEpoch)
	if err != nil {
		return ws, err
	}
	var globalSum float64
	var globalN, calmN int64
	var peakGust, maxSustained, bestDayAvg *float64
	for _, d := range days {
		globalSum += d.windSum
		globalN += d.windN
		calmN += d.calm
		if d.maxGust != nil && (peakGust == nil || *d.maxGust > *peakGust) {
			v := *d.maxGust
			peakGust = &v
			ws.PeakGustDay = d.day.Format("2006-01-02")
		}
		if d.maxSustained != nil && (maxSustained == nil || *d.maxSustained > *maxSustained) {
			v := *d.maxSustained
			maxSustained = &v
			ws.MaxSustainedDay = d.day.Format("2006-01-02")
		}
		if d.windN > 0 {
			dayAvg := d.windSum / float64(d.windN)
			if bestDayAvg == nil || dayAvg > *bestDayAvg {
				v := dayAvg
				bestDayAvg = &v
				ws.WindiestDay = d.day.Format("2006-01-02")
			}
		}
	}
	ws.Obs = globalN
	if globalN > 0 {
		avg := model.MpsToMph(globalSum / float64(globalN))
		ws.AvgWindMph = &avg
		ws.CalmPct = 100 * float64(calmN) / float64(globalN)
	}
	if peakGust != nil {
		v := model.MpsToMph(*peakGust)
		ws.PeakGustMph = &v
	}
	if maxSustained != nil {
		v := model.MpsToMph(*maxSustained)
		ws.MaxSustainedMph = &v
	}
	if bestDayAvg != nil {
		v := model.MpsToMph(*bestDayAvg)
		ws.WindiestDayAvgMph = &v
	}
	return ws, nil
}

// windDays rolls the observations in [startEpoch, endEpoch] up to local calendar
// days, oldest first, carrying each day's wind sum and count (for the weighted
// average), sustained and gust maxima, and calm count.
func (s *Store) windDays(ctx context.Context, startEpoch, endEpoch int64) (_ []windDay, err error) {
	rows, err := s.queryRange(ctx, startEpoch, endEpoch, qryWindRollup, rollupBucketSeconds, calmThresholdMps, startEpoch, endEpoch)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, archiveFailure("close wind days", rows.Close())) }()

	var out []windDay
	for rows.Next() {
		var b, windN, calm int64
		var windSum float64
		var maxAvg, maxGust sql.NullFloat64
		if err := rows.Scan(&b, &windSum, &windN, &maxAvg, &maxGust, &calm); err != nil {
			return nil, archiveFailure("scan wind day", err)
		}
		t := time.Unix(b*rollupBucketSeconds, 0).Local()
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
		if len(out) == 0 || !out[len(out)-1].day.Equal(day) {
			out = append(out, windDay{day: day})
		}
		cur := &out[len(out)-1]
		cur.windSum += windSum
		cur.windN += windN
		cur.calm += calm
		cur.maxSustained = maxPtr(cur.maxSustained, nf(maxAvg))
		cur.maxGust = maxPtr(cur.maxGust, nf(maxGust))
	}
	return out, archiveFailure("read wind days", rows.Err())
}

// solarDays rolls the observations in [startEpoch, endEpoch] up to local calendar
// days, oldest first, integrating each 15-minute bucket's mean irradiance over
// its span into daily insolation (see rollupBucketSeconds for the exact-day
// argument; a bucket never straddles a local day).
func (s *Store) solarDays(ctx context.Context, startEpoch, endEpoch int64) (_ []solarDay, err error) {
	rows, err := s.queryRange(ctx, startEpoch, endEpoch, qrySolarRollup, rollupBucketSeconds, startEpoch, endEpoch)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, archiveFailure("close solar days", rows.Close())) }()

	var out []solarDay
	for rows.Next() {
		var b, solarN int64
		var avgSolar, maxSolar, maxUV, maxLux sql.NullFloat64
		if err := rows.Scan(&b, &avgSolar, &maxSolar, &maxUV, &maxLux, &solarN); err != nil {
			return nil, archiveFailure("scan solar day", err)
		}
		t := time.Unix(b*rollupBucketSeconds, 0).Local()
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
		if len(out) == 0 || !out[len(out)-1].day.Equal(day) {
			out = append(out, solarDay{day: day})
		}
		cur := &out[len(out)-1]
		if avgSolar.Valid {
			// Piecewise-constant integration: the bucket mean (W/m²) held over its
			// span (rollupBucketSeconds), in MJ/m² (÷1e6 J→MJ).
			cur.energyMJ += avgSolar.Float64 * rollupBucketSeconds / 1e6
			cur.hasEnergy = true
		}
		cur.peakWm2 = maxPtr(cur.peakWm2, nf(maxSolar))
		cur.peakUV = maxPtr(cur.peakUV, nf(maxUV))
		cur.peakLux = maxPtr(cur.peakLux, nf(maxLux))
	}
	return out, archiveFailure("read solar days", rows.Err())
}

// lightningDays rolls the observations in [startEpoch, endEpoch] up to local
// calendar days, oldest first, following the same integer-bucket path as
// dayAggregates (see rollupBucketSeconds for why the day assignment is exact).
func (s *Store) lightningDays(ctx context.Context, startEpoch, endEpoch int64) (_ []lightningDay, err error) {
	rows, err := s.queryRange(ctx, startEpoch, endEpoch, qryLightningRollup, rollupBucketSeconds, startEpoch, endEpoch)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, archiveFailure("close lightning days", rows.Close())) }()

	var out []lightningDay
	for rows.Next() {
		var b, strikes, obs int64
		var closest, farthest sql.NullFloat64
		if err := rows.Scan(&b, &strikes, &closest, &farthest, &obs); err != nil {
			return nil, archiveFailure("scan lightning day", err)
		}
		t := time.Unix(b*rollupBucketSeconds, 0).Local()
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
		if len(out) == 0 || !out[len(out)-1].day.Equal(day) {
			out = append(out, lightningDay{day: day})
		}
		cur := &out[len(out)-1]
		cur.strikes += strikes
		cur.closestKm = minPtr(cur.closestKm, nf(closest))
	}
	return out, archiveFailure("read lightning days", rows.Err())
}
