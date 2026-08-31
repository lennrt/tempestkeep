package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

// calmThresholdMps is the average wind speed below which an observation counts
// as calm (direction is meaningless when the air is still): 0.5 m/s ≈ 1.1 mph,
// the bottom of Beaufort 1.
const calmThresholdMps = 0.5

// WindSector is one 16-point compass sector of a wind rose. Pct is the share of
// non-calm observations whose direction fell in this sector.
type WindSector struct {
	Sector     string   `json:"sector"` // N, NNE, NE, ... NNW
	Count      int64    `json:"count"`
	Pct        float64  `json:"pct"`
	AvgMph     *float64 `json:"avg_mph,omitempty"`
	MaxGustMph *float64 `json:"max_gust_mph,omitempty"`
}

// WindRose is the distribution of wind over the 16 compass sectors. Sectors
// always has all 16 entries, N first, including zero-count sectors, so the
// result renders directly as a rose.
type WindRose struct {
	Sectors          []WindSector `json:"sectors"`
	CalmPct          float64      `json:"calm_pct"` // share of wind observations below the calm threshold
	CalmThresholdMph float64      `json:"calm_threshold_mph"`
	Obs              int64        `json:"obs"` // observations carrying wind data
}

// WindRose bins the observations in [startEpoch, endEpoch] into 16 compass
// sectors by wind direction, with per-sector mean speed and peak gust. Calm
// observations (below calmThresholdMps) are reported as CalmPct rather than
// binned, since their direction carries no signal.
func (s *Store) WindRose(ctx context.Context, startEpoch, endEpoch int64) (_ WindRose, err error) {
	rose := WindRose{
		Sectors:          make([]WindSector, 16),
		CalmThresholdMph: model.MpsToMph(calmThresholdMps),
	}
	for i := range rose.Sectors {
		rose.Sectors[i].Sector = model.Compass(float64(i) * 22.5)
	}

	if err := validateEpochRange(startEpoch, endEpoch); err != nil {
		return rose, err
	}
	db, err := s.database()
	if err != nil {
		return rose, err
	}
	rows, err := db.QueryContext(ctx, qryWindRoseSectors, startEpoch, endEpoch, calmThresholdMps)
	if err != nil {
		return rose, archiveFailure("read wind rose", err)
	}
	defer func() { err = errors.Join(err, archiveFailure("close wind rose", rows.Close())) }()

	var windy int64
	for rows.Next() {
		var idx int
		var count int64
		var avg, gust sql.NullFloat64
		if err := rows.Scan(&idx, &count, &avg, &gust); err != nil {
			return rose, archiveFailure("scan wind rose", err)
		}
		if idx < 0 || idx >= len(rose.Sectors) {
			return rose, fmt.Errorf("%w: wind sector %d is outside 0..15", ErrInvalidArchive, idx)
		}
		sec := &rose.Sectors[idx]
		sec.Count = count
		sec.AvgMph = mpsToMphPtr(avg)
		sec.MaxGustMph = mpsToMphPtr(gust)
		windy += count
	}
	if err := rows.Err(); err != nil {
		return rose, archiveFailure("read wind rose", err)
	}

	var calm int64
	if err := db.QueryRowContext(ctx, qryWindRoseCalm,
		startEpoch, endEpoch, calmThresholdMps).Scan(&calm); err != nil {
		return rose, archiveFailure("read calm wind share", err)
	}

	rose.Obs = windy + calm
	if rose.Obs > 0 {
		rose.CalmPct = 100 * float64(calm) / float64(rose.Obs)
	}
	if windy > 0 {
		for i := range rose.Sectors {
			rose.Sectors[i].Pct = 100 * float64(rose.Sectors[i].Count) / float64(windy)
		}
	}
	return rose, nil
}

// SeriesPoint is one time bucket of a downsampled observation series, in US
// display units. Aggregation per field matches its physics: temperatures keep
// avg/min/max, gust and UV keep the max, rain and lightning strikes sum.
type SeriesPoint struct {
	Epoch        int64    `json:"epoch"` // bucket start, unix seconds
	Time         string   `json:"time"`  // bucket start, local RFC3339
	TempAvgF     *float64 `json:"temp_avg_f,omitempty"`
	TempMinF     *float64 `json:"temp_min_f,omitempty"`
	TempMaxF     *float64 `json:"temp_max_f,omitempty"`
	HumidityPct  *float64 `json:"humidity_pct,omitempty"`
	PressureInHg *float64 `json:"pressure_inhg,omitempty"`
	WindMph      *float64 `json:"wind_mph,omitempty"`
	GustMph      *float64 `json:"gust_mph,omitempty"`
	RainIn       float64  `json:"rain_in"`
	UVMax        *float64 `json:"uv_max,omitempty"`
	SolarAvgWm2  *float64 `json:"solar_avg_wm2,omitempty"`
	Strikes      int64    `json:"lightning_strikes"`
	Obs          int64    `json:"obs"`
}

// Series returns the observations in [startEpoch, endEpoch] downsampled into
// fixed buckets of bucketSeconds (aligned to the unix epoch), oldest first.
// Buckets with no observations are absent rather than zero-filled.
func (s *Store) Series(ctx context.Context, startEpoch, endEpoch, bucketSeconds int64) (_ []SeriesPoint, err error) {
	if err := validateEpochRange(startEpoch, endEpoch); err != nil {
		return nil, err
	}
	if bucketSeconds == 0 {
		bucketSeconds = 60
	}
	if bucketSeconds < 60 || bucketSeconds > MaxRangeSeconds {
		return nil, fmt.Errorf("%w: series bucket must be between 60 and %d seconds", ErrInvalidArgument, MaxRangeSeconds)
	}
	points := (endEpoch-startEpoch)/bucketSeconds + 1
	if points > MaxSeriesPoints {
		return nil, fmt.Errorf("%w: series can contain at most %d buckets", ErrInvalidArgument, MaxSeriesPoints)
	}
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, qrySeries,
		bucketSeconds, bucketSeconds, startEpoch, endEpoch)
	if err != nil {
		return nil, archiveFailure("read observation series", err)
	}
	defer func() { err = errors.Join(err, archiveFailure("close observation series", rows.Close())) }()

	var out []SeriesPoint
	for rows.Next() {
		var p SeriesPoint
		var tavg, tmin, tmax, hum, pmb, wind, gust, uv, solar sql.NullFloat64
		var rainMm, strikes float64
		if err := rows.Scan(&p.Epoch, &tavg, &tmin, &tmax, &hum, &pmb, &wind, &gust,
			&rainMm, &uv, &solar, &strikes, &p.Obs); err != nil {
			return nil, archiveFailure("scan observation series", err)
		}
		p.Time = localTime(p.Epoch)
		p.TempAvgF, p.TempMinF, p.TempMaxF = cToFPtr(tavg), cToFPtr(tmin), cToFPtr(tmax)
		p.HumidityPct = nf(hum)
		if pmb.Valid {
			v := model.MbToInHg(pmb.Float64)
			p.PressureInHg = &v
		}
		p.WindMph, p.GustMph = mpsToMphPtr(wind), mpsToMphPtr(gust)
		p.RainIn = model.MmToInch(rainMm)
		p.UVMax = nf(uv)
		p.SolarAvgWm2 = nf(solar)
		p.Strikes = int64(strikes)
		out = append(out, p)
	}
	return out, archiveFailure("read observation series", rows.Err())
}
