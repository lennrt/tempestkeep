package store

// History queries aggregate indexed rows through the read-only connection.
// They convert display results to US units.

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

// YearDay is one calendar year's aggregates for a single month/day (one row of
// a "this day in history" answer), in US display units.
type YearDay struct {
	Year        int      `json:"year"`
	Day         string   `json:"day"` // YYYY-MM-DD, local time
	TempMinF    *float64 `json:"temp_min_f,omitempty"`
	TempMaxF    *float64 `json:"temp_max_f,omitempty"`
	TempAvgF    *float64 `json:"temp_avg_f,omitempty"`
	RainIn      float64  `json:"rain_in"`
	PeakGustMph *float64 `json:"peak_gust_mph,omitempty"`
	Obs         int64    `json:"obs"`
}

// ThisDay returns the aggregates for one month/day across every year in the
// archive, oldest first. Years with no observations on that day are absent, and
// Feb 29 skips non-leap years. Each year is one indexed range aggregate
// over that local calendar day, never a full-table scan.
func (s *Store) ThisDay(ctx context.Context, month, day int) ([]YearDay, error) {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return nil, fmt.Errorf("%w: month/day %02d-%02d", ErrInvalidArgument, month, day)
	}
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	cov, err := s.Coverage(ctx)
	if err != nil {
		return nil, err
	}
	if !cov.MinEpoch.Valid {
		return nil, nil // empty archive
	}
	firstYear := time.Unix(cov.MinEpoch.Int64, 0).Local().Year()
	lastYear := time.Unix(cov.MaxEpoch.Int64, 0).Local().Year()

	var out []YearDay
	for y := firstYear; y <= lastYear; y++ {
		start := time.Date(y, time.Month(month), day, 0, 0, 0, 0, time.Local)
		if int(start.Month()) != month || start.Day() != day {
			continue // the date doesn't exist this year (Feb 29)
		}
		end := start.AddDate(0, 0, 1)

		var tmin, tmax, gust sql.NullFloat64
		var tempSum, rainMm float64
		var tempN, obs int64
		if err := db.QueryRowContext(ctx, qryThisDay, start.Unix(), end.Unix()).
			Scan(&tmin, &tmax, &tempSum, &tempN, &rainMm, &gust, &obs); err != nil {
			return nil, archiveFailure("read this-day history", err)
		}
		if obs == 0 {
			continue
		}
		yd := YearDay{
			Year: y, Day: start.Format("2006-01-02"), Obs: obs,
			RainIn: model.MmToInch(rainMm),
		}
		yd.TempMinF, yd.TempMaxF = cToFPtr(tmin), cToFPtr(tmax)
		if tempN > 0 {
			v := model.CToF(tempSum / float64(tempN))
			yd.TempAvgF = &v
		}
		yd.PeakGustMph = mpsToMphPtr(gust)
		out = append(out, yd)
	}
	return out, nil
}

// PeriodStat is one calendar month or year of aggregates, in US display units.
// TempAvgF is the mean over every observation in the period (not a mean of
// daily means); a rainy day is one with at least 0.01 in (0.254 mm) of rain.
type PeriodStat struct {
	Period       string   `json:"period"` // YYYY-MM or YYYY, local time
	TempMinF     *float64 `json:"temp_min_f,omitempty"`
	TempMaxF     *float64 `json:"temp_max_f,omitempty"`
	TempAvgF     *float64 `json:"temp_avg_f,omitempty"`
	RainIn       float64  `json:"rain_in"`
	PeakGustMph  *float64 `json:"peak_gust_mph,omitempty"`
	DaysObserved int64    `json:"days_observed"`
	RainyDays    int64    `json:"rainy_days"`
	Obs          int64    `json:"obs"`
}

// rainyDayMm is the daily rainfall at or above which a day counts as rainy:
// 0.254 mm, the standard 0.01 in "measurable precipitation" threshold.
const rainyDayMm = 0.254

// Period selects the calendar bucket for PeriodSummary.
type Period uint8

const (
	PeriodMonth Period = iota + 1
	PeriodYear
)

// PeriodSummary aggregates observations within [startEpoch, endEpoch] by local
// calendar month or year, oldest first. It builds
// on the shared day rollup (see dayAggregates), which is also what lets it
// report days observed and rainy days per period.
func (s *Store) PeriodSummary(ctx context.Context, period Period, startEpoch, endEpoch int64) ([]PeriodStat, error) {
	var layout string
	switch period {
	case PeriodMonth:
		layout = "2006-01"
	case PeriodYear:
		layout = "2006"
	default:
		return nil, fmt.Errorf("%w: unsupported summary period %d", ErrInvalidArgument, period)
	}
	days, err := s.dayAggregates(ctx, startEpoch, endEpoch)
	if err != nil {
		return nil, err
	}

	var out []PeriodStat
	var cur *periodAcc
	flush := func() {
		if cur != nil {
			out = append(out, cur.stat())
			cur = nil
		}
	}
	for _, d := range days {
		key := d.day.Format(layout)
		if cur == nil || cur.period != key {
			flush()
			cur = &periodAcc{period: key}
		}
		cur.add(d)
	}
	flush()
	return out, nil
}

// periodAcc accumulates day aggregates into one calendar period.
type periodAcc struct {
	period  string
	tempMin *float64
	tempMax *float64
	tempSum float64
	tempN   int64
	rainMm  float64
	gustMax *float64
	days    int64
	rainy   int64
	obs     int64
}

func (p *periodAcc) add(d dayAgg) {
	p.tempMin = minPtr(p.tempMin, d.tempMin)
	p.tempMax = maxPtr(p.tempMax, d.tempMax)
	p.tempSum += d.tempSum
	p.tempN += d.tempN
	p.rainMm += d.rainMm
	p.gustMax = maxPtr(p.gustMax, d.gustMax)
	p.days++
	if d.rainMm >= rainyDayMm {
		p.rainy++
	}
	p.obs += d.obs
}

func (p *periodAcc) stat() PeriodStat {
	ps := PeriodStat{
		Period: p.period, RainIn: model.MmToInch(p.rainMm),
		DaysObserved: p.days, RainyDays: p.rainy, Obs: p.obs,
	}
	if p.tempMin != nil {
		v := model.CToF(*p.tempMin)
		ps.TempMinF = &v
	}
	if p.tempMax != nil {
		v := model.CToF(*p.tempMax)
		ps.TempMaxF = &v
	}
	if p.tempN > 0 {
		v := model.CToF(p.tempSum / float64(p.tempN))
		ps.TempAvgF = &v
	}
	if p.gustMax != nil {
		v := model.MpsToMph(*p.gustMax)
		ps.PeakGustMph = &v
	}
	return ps
}

// MonthNormal is one calendar month's climate normal: the average conditions for
// that month across every year the archive covers, in US display units. The
// temperature normals average the per-year monthly means and extremes; the rain
// normals average the per-year monthly totals and rainy-day counts.
type MonthNormal struct {
	Month        int      `json:"month"`      // 1-12
	MonthName    string   `json:"month_name"` // January ... December
	Years        int64    `json:"years"`      // years that contributed a value
	TempAvgF     *float64 `json:"temp_avg_f,omitempty"`
	TempMinF     *float64 `json:"temp_min_f,omitempty"` // average of the monthly lows
	TempMaxF     *float64 `json:"temp_max_f,omitempty"` // average of the monthly highs
	AvgRainIn    float64  `json:"avg_rain_in"`
	AvgRainyDays float64  `json:"avg_rainy_days"`
}

// monthNormalAcc accumulates one calendar month across years.
type monthNormalAcc struct {
	tempAvgSum, tempMinSum, tempMaxSum float64
	tempAvgN, tempMinN, tempMaxN       int64
	rainSum, rainySum                  float64
	years                              int64
}

// MonthlyNormals computes the climate normal for each calendar month from every
// year in the archive: the average temperature, average monthly low and high,
// average monthly rainfall, and average rainy days. Months the archive never
// observed are omitted. It builds on the monthly PeriodSummary, so each year's
// month contributes one value to its normal.
func (s *Store) MonthlyNormals(ctx context.Context) ([]MonthNormal, error) {
	start, end, ok, err := s.archiveRange(ctx)
	if err != nil || !ok {
		return nil, err
	}
	months, err := s.PeriodSummary(ctx, PeriodMonth, start, end)
	if err != nil {
		return nil, err
	}
	return monthlyNormalsFrom(months), nil
}

// monthlyNormalsFrom folds a monthly PeriodSummary into the 12 calendar-month
// normals. It's split out so callers that already hold the monthly summary (e.g.
// TemperatureTrend) can reuse it without a second scan of the archive.
func monthlyNormalsFrom(months []PeriodStat) []MonthNormal {
	var accs [12]monthNormalAcc
	for _, m := range months {
		t, err := time.Parse("2006-01", m.Period)
		if err != nil {
			continue // defensive: PeriodSummary always emits YYYY-MM
		}
		a := &accs[int(t.Month())-1]
		a.years++
		if m.TempAvgF != nil {
			a.tempAvgSum += *m.TempAvgF
			a.tempAvgN++
		}
		if m.TempMinF != nil {
			a.tempMinSum += *m.TempMinF
			a.tempMinN++
		}
		if m.TempMaxF != nil {
			a.tempMaxSum += *m.TempMaxF
			a.tempMaxN++
		}
		a.rainSum += m.RainIn
		a.rainySum += float64(m.RainyDays)
	}

	var out []MonthNormal
	for i := range 12 {
		a := accs[i]
		if a.years == 0 {
			continue
		}
		mn := MonthNormal{
			Month: i + 1, MonthName: time.Month(i + 1).String(), Years: a.years,
			AvgRainIn:    a.rainSum / float64(a.years),
			AvgRainyDays: a.rainySum / float64(a.years),
		}
		if a.tempAvgN > 0 {
			v := a.tempAvgSum / float64(a.tempAvgN)
			mn.TempAvgF = &v
		}
		if a.tempMinN > 0 {
			v := a.tempMinSum / float64(a.tempMinN)
			mn.TempMinF = &v
		}
		if a.tempMaxN > 0 {
			v := a.tempMaxSum / float64(a.tempMaxN)
			mn.TempMaxF = &v
		}
		out = append(out, mn)
	}
	return out
}

// TempTrend is a least-squares warming/cooling trend fitted to monthly
// temperature anomalies, each month's mean minus that calendar month's normal.
// Working in anomalies removes the seasonal cycle, so the slope reflects the
// underlying trend rather than which months the archive happens to cover, and a
// partial year contributes each of its months against that month's own normal.
type TempTrend struct {
	Years      int64  `json:"years"`                 // distinct calendar years with a usable month
	MonthsUsed int64  `json:"months_used"`           // month-anomalies that fed the fit
	FirstMonth string `json:"first_month,omitempty"` // YYYY-MM
	LastMonth  string `json:"last_month,omitempty"`

	// SlopePerDecadeF is the fitted warming rate in °F per decade; positive is
	// warming. Nil until the anomalies actually vary over time, which needs the
	// same calendar month observed in more than one year (a single year, or years
	// that never share a month, leave every anomaly at zero and carry no trend).
	SlopePerDecadeF *float64 `json:"slope_per_decade_f,omitempty"`
	RSquared        *float64 `json:"r_squared,omitempty"` // fit quality, 0..1

	WarmestAnomalyF *float64 `json:"warmest_anomaly_f,omitempty"`
	WarmestMonth    string   `json:"warmest_month,omitempty"`
	ColdestAnomalyF *float64 `json:"coldest_anomaly_f,omitempty"`
	ColdestMonth    string   `json:"coldest_month,omitempty"`
}

// TemperatureTrend fits a linear trend to the whole archive's monthly temperature
// anomalies (each month's mean against its calendar-month normal) and reports the
// slope in °F per decade, the fit's R², and the warmest and coldest months
// relative to normal. It builds on the monthly PeriodSummary and MonthlyNormals,
// so the anomaly baseline is the archive's own climatology. A short archive
// (under two calendar years) yields no slope, since one year of data defines its
// own normals and every anomaly collapses to zero.
func (s *Store) TemperatureTrend(ctx context.Context) (TempTrend, error) {
	start, end, ok, err := s.archiveRange(ctx)
	if err != nil || !ok {
		return TempTrend{}, err
	}
	months, err := s.PeriodSummary(ctx, PeriodMonth, start, end)
	if err != nil {
		return TempTrend{}, err
	}
	// Reuse the one monthly summary for both the anomaly series and its baseline,
	// so the whole trend costs a single archive scan.
	normals := monthlyNormalsFrom(months)
	var normAvg [13]*float64 // indexed by calendar month 1..12
	for _, n := range normals {
		normAvg[n.Month] = n.TempAvgF
	}

	var tt TempTrend
	var sumX, sumY, sumXY, sumXX, sumYY float64
	years := map[int]bool{}
	for _, m := range months {
		if m.TempAvgF == nil {
			continue
		}
		t, perr := time.Parse("2006-01", m.Period)
		if perr != nil {
			continue // defensive: PeriodSummary always emits YYYY-MM
		}
		na := normAvg[int(t.Month())]
		if na == nil {
			continue
		}
		anom := *m.TempAvgF - *na
		// Decimal year at mid-month keeps the x-axis in years, so the slope is
		// directly °F/year (×10 for the reported per-decade rate).
		x := float64(t.Year()) + (float64(t.Month())-0.5)/12

		sumX += x
		sumY += anom
		sumXY += x * anom
		sumXX += x * x
		sumYY += anom * anom
		tt.MonthsUsed++
		years[t.Year()] = true
		if tt.FirstMonth == "" {
			tt.FirstMonth = m.Period
		}
		tt.LastMonth = m.Period
		if tt.WarmestAnomalyF == nil || anom > *tt.WarmestAnomalyF {
			v := anom
			tt.WarmestAnomalyF = &v
			tt.WarmestMonth = m.Period
		}
		if tt.ColdestAnomalyF == nil || anom < *tt.ColdestAnomalyF {
			v := anom
			tt.ColdestAnomalyF = &v
			tt.ColdestMonth = m.Period
		}
	}
	tt.Years = int64(len(years))

	if len(years) >= 2 {
		n := float64(tt.MonthsUsed)
		denom := n*sumXX - sumX*sumX   // variance in time
		spreadY := n*sumYY - sumY*sumY // variance in the anomalies
		// Both must vary to fit a line. spreadY is zero when every anomaly is
		// identical (all zero), which happens when no calendar month is observed in
		// more than one year (each month is then its own normal); report no slope
		// rather than a meaningless zero.
		if denom > 0 && spreadY > 0 {
			cov := n*sumXY - sumX*sumY
			perDecade := (cov / denom) * 10
			tt.SlopePerDecadeF = &perDecade
			r2 := (cov * cov) / (denom * spreadY) // Pearson R²
			tt.RSquared = &r2
		}
	}
	return tt, nil
}
