package store

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

// DegreeDayParams configures a degree-day accumulation. Bases and the growing
// cap are in °F, the unit US utilities and growers state them in.
type DegreeDayParams struct {
	HeatingBaseF float64 // HDD base (US convention: 65°F)
	CoolingBaseF float64 // CDD base (US convention: 65°F)
	GrowingBaseF float64 // GDD lower base (e.g. 50°F for corn)
	GrowingCapF  float64 // GDD upper cap on the daily high; <=0 disables capping
}

// DegreeDayStat is the degree-day accumulation over one bucket (a calendar
// month, or the whole range when Period is empty), in °F-days.
type DegreeDayStat struct {
	Period string  `json:"period,omitempty"` // YYYY-MM; empty on the range total
	HDD    float64 `json:"heating_degree_days"`
	CDD    float64 `json:"cooling_degree_days"`
	GDD    float64 `json:"growing_degree_days"`
	Days   int64   `json:"days"` // days that carried both a high and a low
}

// DegreeDays accumulates heating, cooling, and growing degree-days over
// [startEpoch, endEpoch], returning the whole-range total plus a per-month
// breakdown (oldest first). Each day's mean is the meteorological midpoint
// (dailyHigh+dailyLow)/2, the standard degree-day basis; a day missing either
// extreme is skipped rather than counted with a biased mean. Growing
// degree-days use the modified method NOAA and most growers apply: the high is
// capped at GrowingCapF and the low floored at GrowingBaseF before the midpoint,
// so heat past the cap and cold below the base neither add nor subtract growth.
func (s *Store) DegreeDays(ctx context.Context, p DegreeDayParams, startEpoch, endEpoch int64) (DegreeDayStat, []DegreeDayStat, error) {
	for _, threshold := range []struct {
		name  string
		value float64
	}{
		{"heating base", p.HeatingBaseF},
		{"cooling base", p.CoolingBaseF},
		{"growing base", p.GrowingBaseF},
		{"growing cap", p.GrowingCapF},
	} {
		if err := validateTemperatureF(threshold.name, threshold.value); err != nil {
			return DegreeDayStat{}, nil, err
		}
	}
	if p.GrowingCapF > 0 && p.GrowingCapF < p.GrowingBaseF {
		return DegreeDayStat{}, nil, fmt.Errorf("%w: growing cap must not be less than growing base", ErrInvalidArgument)
	}
	days, err := s.dayAggregates(ctx, startEpoch, endEpoch)
	if err != nil {
		return DegreeDayStat{}, nil, err
	}
	var total DegreeDayStat
	var months []DegreeDayStat
	var cur *DegreeDayStat
	for _, d := range days {
		if d.tempMin == nil || d.tempMax == nil {
			continue
		}
		hi, lo := model.CToF(*d.tempMax), model.CToF(*d.tempMin)
		mean := (hi + lo) / 2
		hdd := math.Max(0, p.HeatingBaseF-mean)
		cdd := math.Max(0, mean-p.CoolingBaseF)

		gHi := hi
		if p.GrowingCapF > 0 && gHi > p.GrowingCapF {
			gHi = p.GrowingCapF
		}
		gLo := math.Max(lo, p.GrowingBaseF)
		gdd := math.Max(0, (gHi+gLo)/2-p.GrowingBaseF)

		key := d.day.Format("2006-01")
		if cur == nil || cur.Period != key {
			months = append(months, DegreeDayStat{Period: key})
			cur = &months[len(months)-1]
		}
		cur.HDD += hdd
		cur.CDD += cdd
		cur.GDD += gdd
		cur.Days++
		total.HDD += hdd
		total.CDD += cdd
		total.GDD += gdd
		total.Days++
	}
	return total, months, nil
}

// ClimateIndexParams sets the thresholds for the threshold-day counts. All are
// in °F; each zero field falls back to the WMO/ETCCDI-equivalent default.
type ClimateIndexParams struct {
	FrostMaxF         float64 // frost day: daily low below this (default 32°F / 0°C)
	IceMaxF           float64 // ice day: daily high stays below this (default 32°F)
	SummerMinF        float64 // summer day: daily high reaches this (default 77°F / 25°C)
	HotMinF           float64 // hot day: daily high reaches this (default 90°F)
	TropicalNightMinF float64 // tropical night: daily low stays at or above this (default 68°F / 20°C)
}

// WithDefaults fills each zero threshold with its standard value, so callers can
// report the thresholds actually applied.
func (p ClimateIndexParams) WithDefaults() ClimateIndexParams {
	if p.FrostMaxF == 0 {
		p.FrostMaxF = 32
	}
	if p.IceMaxF == 0 {
		p.IceMaxF = 32
	}
	if p.SummerMinF == 0 {
		p.SummerMinF = 77
	}
	if p.HotMinF == 0 {
		p.HotMinF = 90
	}
	if p.TropicalNightMinF == 0 {
		p.TropicalNightMinF = 68
	}
	return p
}

// ClimateIndexStat is one calendar year of threshold-day counts (or the whole
// range when Period is empty). Each count considers only days that carried both
// a high and a low, reported as Days.
type ClimateIndexStat struct {
	Period         string `json:"period,omitempty"` // YYYY; empty on the range total
	FrostDays      int64  `json:"frost_days"`
	IceDays        int64  `json:"ice_days"`
	SummerDays     int64  `json:"summer_days"`
	HotDays        int64  `json:"hot_days"`
	TropicalNights int64  `json:"tropical_nights"`
	Days           int64  `json:"days"`
}

// ClimateIndices counts the standard threshold days over [startEpoch, endEpoch],
// returning the range total plus a per-year breakdown (oldest first): frost days
// (low below FrostMaxF), ice days (high below IceMaxF), summer days (high at or
// above SummerMinF), hot days (high at or above HotMinF), and tropical nights
// (low at or above TropicalNightMinF). Days missing a high or a low are skipped.
func (s *Store) ClimateIndices(ctx context.Context, p ClimateIndexParams, startEpoch, endEpoch int64) (ClimateIndexStat, []ClimateIndexStat, error) {
	p = p.WithDefaults()
	for _, threshold := range []struct {
		name  string
		value float64
	}{
		{"frost maximum", p.FrostMaxF},
		{"ice maximum", p.IceMaxF},
		{"summer minimum", p.SummerMinF},
		{"hot minimum", p.HotMinF},
		{"tropical night minimum", p.TropicalNightMinF},
	} {
		if err := validateTemperatureF(threshold.name, threshold.value); err != nil {
			return ClimateIndexStat{}, nil, err
		}
	}
	days, err := s.dayAggregates(ctx, startEpoch, endEpoch)
	if err != nil {
		return ClimateIndexStat{}, nil, err
	}
	var total ClimateIndexStat
	var years []ClimateIndexStat
	var cur *ClimateIndexStat
	for _, d := range days {
		if d.tempMin == nil || d.tempMax == nil {
			continue
		}
		hi, lo := model.CToF(*d.tempMax), model.CToF(*d.tempMin)

		key := d.day.Format("2006")
		if cur == nil || cur.Period != key {
			years = append(years, ClimateIndexStat{Period: key})
			cur = &years[len(years)-1]
		}
		bump := func(f func(*ClimateIndexStat)) { f(cur); f(&total) }
		bump(func(c *ClimateIndexStat) { c.Days++ })
		if lo < p.FrostMaxF {
			bump(func(c *ClimateIndexStat) { c.FrostDays++ })
		}
		if hi < p.IceMaxF {
			bump(func(c *ClimateIndexStat) { c.IceDays++ })
		}
		if hi >= p.SummerMinF {
			bump(func(c *ClimateIndexStat) { c.SummerDays++ })
		}
		if hi >= p.HotMinF {
			bump(func(c *ClimateIndexStat) { c.HotDays++ })
		}
		if lo >= p.TropicalNightMinF {
			bump(func(c *ClimateIndexStat) { c.TropicalNights++ })
		}
	}
	return total, years, nil
}

// TempSpellParams sets the thresholds for the temperature spells, in °F. A zero
// field falls back to its standard default.
type TempSpellParams struct {
	HeatMinF float64 // heat-wave day: daily high at or above this (default 90°F)
	ColdMaxF float64 // cold-snap day: daily low at or below this (default 32°F)
}

// WithDefaults fills each zero threshold with its standard value, so callers can
// report the thresholds actually applied.
func (p TempSpellParams) WithDefaults() TempSpellParams {
	if p.HeatMinF == 0 {
		p.HeatMinF = 90
	}
	if p.ColdMaxF == 0 {
		p.ColdMaxF = 32
	}
	return p
}

// TempSpells is the longest hot and cold streaks over a date range. A heat-wave
// day has a daily high at or above HeatMinF; a cold-snap day has a daily low at
// or below ColdMaxF. Streaks run over consecutive observed calendar days, so a
// coverage gap (or a day missing the relevant extreme) ends the run rather than
// bridging it, matching the rain-spell logic.
type TempSpells struct {
	HeatThresholdF float64 `json:"heat_threshold_f"`
	ColdThresholdF float64 `json:"cold_threshold_f"`
	DaysObserved   int64   `json:"days_observed"`

	LongestHeatWaveDays int64  `json:"longest_heat_wave_days"`
	HeatWaveStart       string `json:"heat_wave_start,omitempty"`
	HeatWaveEnd         string `json:"heat_wave_end,omitempty"`

	LongestColdSnapDays int64  `json:"longest_cold_snap_days"`
	ColdSnapStart       string `json:"cold_snap_start,omitempty"`
	ColdSnapEnd         string `json:"cold_snap_end,omitempty"`
}

// TemperatureSpells finds the longest heat wave and cold snap over
// [startEpoch, endEpoch]: the longest run of consecutive days whose high reaches
// HeatMinF, and whose low reaches ColdMaxF. A day missing the relevant extreme,
// or a gap in coverage, ends the current run, so a streak is never claimed across
// days the archive never recorded.
func (s *Store) TemperatureSpells(ctx context.Context, p TempSpellParams, startEpoch, endEpoch int64) (TempSpells, error) {
	p = p.WithDefaults()
	if err := validateTemperatureF("heat minimum", p.HeatMinF); err != nil {
		return TempSpells{}, err
	}
	if err := validateTemperatureF("cold maximum", p.ColdMaxF); err != nil {
		return TempSpells{}, err
	}
	days, err := s.dayAggregates(ctx, startEpoch, endEpoch)
	if err != nil {
		return TempSpells{}, err
	}
	ts := TempSpells{HeatThresholdF: p.HeatMinF, ColdThresholdF: p.ColdMaxF}
	var heatLen, coldLen int64
	var prev time.Time
	havePrev := false

	fmtDay := func(t time.Time) string { return t.Format("2006-01-02") }
	for _, d := range days {
		ts.DaysObserved++
		consecutive := havePrev && d.day.Equal(prev.AddDate(0, 0, 1))
		if !consecutive {
			heatLen, coldLen = 0, 0 // first day or a coverage gap: start fresh
		}

		if d.tempMax != nil && model.CToF(*d.tempMax) >= p.HeatMinF {
			heatLen++
			if heatLen > ts.LongestHeatWaveDays {
				ts.LongestHeatWaveDays = heatLen
				ts.HeatWaveEnd = fmtDay(d.day)
				ts.HeatWaveStart = fmtDay(d.day.AddDate(0, 0, int(-(heatLen - 1))))
			}
		} else {
			heatLen = 0
		}

		if d.tempMin != nil && model.CToF(*d.tempMin) <= p.ColdMaxF {
			coldLen++
			if coldLen > ts.LongestColdSnapDays {
				ts.LongestColdSnapDays = coldLen
				ts.ColdSnapEnd = fmtDay(d.day)
				ts.ColdSnapStart = fmtDay(d.day.AddDate(0, 0, int(-(coldLen - 1))))
			}
		} else {
			coldLen = 0
		}
		prev, havePrev = d.day, true
	}
	return ts, nil
}

// RainStats summarizes rainfall over a date range, in US display units, plus the
// longest dry and wet spells. A "rainy day" clears the 0.01 in threshold; a
// spell is a run of consecutive observed calendar days, and a missing day (a
// coverage gap) breaks the run rather than silently bridging it.
type RainStats struct {
	TotalIn      float64  `json:"total_in"`
	DaysObserved int64    `json:"days_observed"`
	RainyDays    int64    `json:"rainy_days"`
	WettestDay   string   `json:"wettest_day,omitempty"`
	WettestDayIn *float64 `json:"wettest_day_in,omitempty"`

	LongestDrySpellDays int64  `json:"longest_dry_spell_days"`
	DrySpellStart       string `json:"dry_spell_start,omitempty"`
	DrySpellEnd         string `json:"dry_spell_end,omitempty"`

	LongestWetSpellDays int64  `json:"longest_wet_spell_days"`
	WetSpellStart       string `json:"wet_spell_start,omitempty"`
	WetSpellEnd         string `json:"wet_spell_end,omitempty"`
}

// RainStats aggregates rainfall over [startEpoch, endEpoch]: total, rainy days,
// the wettest day, and the longest dry and wet spells. Spells run over
// consecutive observed calendar days; a gap in coverage ends the current spell,
// so a dry spell is never claimed across days the archive never saw.
func (s *Store) RainStats(ctx context.Context, startEpoch, endEpoch int64) (RainStats, error) {
	days, err := s.dayAggregates(ctx, startEpoch, endEpoch)
	if err != nil {
		return RainStats{}, err
	}
	var rs RainStats
	var bestWetMm float64
	var dryLen, wetLen int64
	var prev time.Time
	havePrev := false

	fmtDay := func(t time.Time) string { return t.Format("2006-01-02") }
	for _, d := range days {
		rs.DaysObserved++
		rs.TotalIn += model.MmToInch(d.rainMm)
		if d.rainMm > bestWetMm {
			bestWetMm = d.rainMm
			rs.WettestDay = fmtDay(d.day)
		}
		rainy := d.rainMm >= rainyDayMm
		if rainy {
			rs.RainyDays++
		}

		if !havePrev || !d.day.Equal(prev.AddDate(0, 0, 1)) {
			dryLen, wetLen = 0, 0 // first day, or a coverage gap: start fresh
		}
		if rainy {
			wetLen++
			dryLen = 0
		} else {
			dryLen++
			wetLen = 0
		}
		if dryLen > rs.LongestDrySpellDays {
			rs.LongestDrySpellDays = dryLen
			rs.DrySpellEnd = fmtDay(d.day)
			rs.DrySpellStart = fmtDay(d.day.AddDate(0, 0, int(-(dryLen - 1))))
		}
		if wetLen > rs.LongestWetSpellDays {
			rs.LongestWetSpellDays = wetLen
			rs.WetSpellEnd = fmtDay(d.day)
			rs.WetSpellStart = fmtDay(d.day.AddDate(0, 0, int(-(wetLen - 1))))
		}
		prev, havePrev = d.day, true
	}
	if bestWetMm > 0 {
		in := model.MmToInch(bestWetMm)
		rs.WettestDayIn = &in
	}
	return rs, nil
}

func validateTemperatureF(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < -200 || value > 300 {
		return fmt.Errorf("%w: %s must be finite and between -200°F and 300°F", ErrInvalidArgument, name)
	}
	return nil
}
