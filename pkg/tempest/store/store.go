// Package store accesses the TempestKeep SQLite archive. Store is read-only.
// Writer creates the schema and appends observations. The package uses a pure-Go
// SQLite driver. Use one archive per device because aggregate queries read all
// rows in the file.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"

	// Registers the pure-Go "sqlite" driver both handles open with.
	_ "modernc.org/sqlite"
)

const (
	maxPathBytes          = 4096
	MaxGapResults         = 1000
	MaxQueryRows          = 1000
	MaxQueryBytes         = 64 << 10
	MaxQueryColumns       = 128
	MaxQueryResultBytes   = 8 << 20
	MaxMetadataKeyBytes   = 128
	MaxMetadataValueBytes = 4096
	MaxInsertBatch        = 10_000
	MaxRangeSeconds       = int64(100 * 366 * 24 * time.Hour / time.Second)
	MaxSeriesPoints       = 2000
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrArchiveNotFound = errors.New("archive not found")
	ErrInvalidArchive  = errors.New("invalid archive")
	ErrDeviceMismatch  = errors.New("archive device mismatch")
	ErrClosed          = errors.New("archive is closed")
	ErrResultTooLarge  = errors.New("query result too large")
	ErrArchiveIO       = errors.New("archive I/O failure")
)

// Store is a read-only archive handle. Its zero value is closed. Close is
// idempotent.
type Store struct {
	db        *sql.DB
	closeOnce sync.Once
	closed    atomic.Bool
	closeErr  error
}

// dsn applies required pragmas to every driver connection.
func dsn(path string, readOnly bool, pragmas ...string) string {
	q := make(url.Values, 1)
	if readOnly {
		q.Set("mode", "ro")
	}
	for _, p := range pragmas {
		q.Add("_pragma", p)
	}
	// Escape URI metacharacters in filenames. Without this, an ordinary path
	// containing '?' or '#' is parsed as DSN options or a fragment and SQLite
	// opens a different file than the one the caller named.
	u := url.URL{Path: filepath.ToSlash(path)}
	return "file:" + u.EscapedPath() + "?" + q.Encode()
}

// Open opens an existing regular archive read-only. It does not create or
// migrate the file. The archive must contain obs_st. The caller owns the result.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Stat first so a missing path is a clear error instead of SQLite quietly
	// creating an empty database file.
	pathInfo, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrArchiveNotFound
		}
		return nil, archiveFailure("inspect archive", err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: archive must be a regular file, not a symlink", ErrInvalidArgument)
	}
	// busy_timeout lets reads wait out a writer's WAL checkpoint instead of
	// failing with a bare "database is locked".
	db, err := sql.Open("sqlite", dsn(path, true, "query_only(1)", "busy_timeout(5000)"))
	if err != nil {
		return nil, archiveFailure("initialize archive reader", err)
	}
	// One connection keeps queries serialized and memory flat; the pragmas
	// above hold on any connection regardless.
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(archiveFailure("open archive", err), archiveFailure("close archive", db.Close()))
	}
	if currentInfo, err := os.Lstat(path); err != nil || !os.SameFile(pathInfo, currentInfo) {
		return nil, errors.Join(
			fmt.Errorf("%w: archive changed while it was opened", ErrInvalidArchive),
			archiveFailure("close archive", db.Close()),
		)
	}
	// Verify the archive actually has the collector's table (also acts as a
	// connectivity check).
	var name string
	switch err := db.QueryRowContext(ctx, qryHasObsTable).Scan(&name); {
	case errors.Is(err, sql.ErrNoRows):
		return nil, errors.Join(
			fmt.Errorf("%w: obs_st table is missing; run the collector first", ErrInvalidArchive),
			archiveFailure("close archive", db.Close()),
		)
	case err != nil:
		return nil, errors.Join(
			archiveFailure("verify archive schema", err),
			archiveFailure("close archive", db.Close()),
		)
	}
	deviceIDs, err := readDeviceIDs(ctx, db)
	if err != nil {
		// Older read-only archives may predate the device_id column. Their
		// aggregate schema is still readable, and without a device column they
		// cannot contain interleaved device ids, so keep supporting them.
		if strings.Contains(err.Error(), "no such column: device_id") {
			return &Store{db: db}, nil
		}
		return nil, errors.Join(
			archiveFailure("read archive devices", err),
			archiveFailure("close archive", db.Close()),
		)
	}
	if len(deviceIDs) > 1 {
		return nil, errors.Join(
			fmt.Errorf("%w: archive contains multiple device IDs; use one archive per device", ErrInvalidArchive),
			archiveFailure("close archive", db.Close()),
		)
	}
	return &Store{db: db}, nil
}

func readDeviceIDs(ctx context.Context, db *sql.DB) (_ []int, err error) {
	rows, err := db.QueryContext(ctx, qryDeviceIDs)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func archiveFailure(action string, err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, os.ErrPermission, os.ErrNotExist} {
		if errors.Is(err, known) {
			return fmt.Errorf("%s: %w", action, known)
		}
	}
	return fmt.Errorf("%w: %s", ErrArchiveIO, action)
}

func validatePath(path string) error {
	if path == "" || len(path) > maxPathBytes || strings.IndexByte(path, 0) >= 0 {
		return fmt.Errorf("%w: archive path must contain 1..%d bytes and no NUL", ErrInvalidArgument, maxPathBytes)
	}
	return nil
}

func validateEpochRange(startEpoch, endEpoch int64) error {
	if startEpoch < 0 || endEpoch < 0 || startEpoch > endEpoch {
		return fmt.Errorf("%w: epoch range must be non-negative and ordered", ErrInvalidArgument)
	}
	if endEpoch-startEpoch > MaxRangeSeconds {
		return fmt.Errorf("%w: epoch range exceeds %d seconds", ErrInvalidArgument, MaxRangeSeconds)
	}
	return nil
}

func (s *Store) queryRange(ctx context.Context, startEpoch, endEpoch int64, query string, args ...any) (*sql.Rows, error) {
	if err := validateEpochRange(startEpoch, endEpoch); err != nil {
		return nil, err
	}
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	return rows, archiveFailure("read archive range", err)
}

func (s *Store) database() (*sql.DB, error) {
	if s == nil || s.db == nil || s.closed.Load() {
		return nil, ErrClosed
	}
	return s.db, nil
}

// Close releases the database handle. Repeated calls return the first result.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		if s.db != nil {
			s.closed.Store(true)
			s.closeErr = archiveFailure("close archive", s.db.Close())
		}
	})
	return s.closeErr
}

// Meta reads a value from the archive's key/value meta table, the read-only
// twin of Writer.Meta, so status tools work without a writable handle. An
// archive without the meta table (not written by this collector) reads as
// having no keys rather than erroring.
func (s *Store) Meta(ctx context.Context, key string) (string, bool, error) {
	if err := validateMetadataKey(key); err != nil {
		return "", false, err
	}
	db, err := s.database()
	if err != nil {
		return "", false, err
	}
	var v string
	switch err := db.QueryRowContext(ctx, qryMetaGet, key).Scan(&v); {
	case err == nil:
		return v, true, nil
	case errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no such table"):
		return "", false, nil
	default:
		return "", false, archiveFailure("read archive metadata", err)
	}
}

func validateMetadataKey(key string) error {
	if key == "" || len(key) > MaxMetadataKeyBytes {
		return fmt.Errorf("%w: metadata key must contain 1..%d bytes", ErrInvalidArgument, MaxMetadataKeyBytes)
	}
	for i := range len(key) {
		b := key[i]
		if (b < 'a' || b > 'z') && (b < '0' || b > '9') && b != '_' && b != '-' && b != '.' {
			return fmt.Errorf("%w: metadata key contains unsupported byte", ErrInvalidArgument)
		}
	}
	return nil
}

// obsColumns is the SELECT list, in the exact order scanObs expects. It is
// shared into latest.sql via template (see queries.go).
const obsColumns = `epoch, wind_lull, wind_avg, wind_gust, wind_dir,
	pressure_mb, air_temp_c, humidity, illuminance_lux, uv, solar_wm2,
	rain_mm, strike_dist_km, strike_count, battery_v`

func nf(n sql.NullFloat64) *float64 {
	if n.Valid {
		v := n.Float64
		return &v
	}
	return nil
}

func scanObs(rows *sql.Rows) (model.Obs, error) {
	var o model.Obs
	var lull, avg, gust, dir, pmb, temp, hum, lux, uv, solar, rain, sdist, scount, batt sql.NullFloat64
	if err := rows.Scan(&o.Epoch, &lull, &avg, &gust, &dir, &pmb, &temp, &hum,
		&lux, &uv, &solar, &rain, &sdist, &scount, &batt); err != nil {
		return o, err
	}
	o.WindLullMps, o.WindAvgMps, o.WindGustMps, o.WindDirDeg = nf(lull), nf(avg), nf(gust), nf(dir)
	o.PressureMb, o.AirTempC, o.HumidityPct = nf(pmb), nf(temp), nf(hum)
	o.IlluminanceLux, o.UV, o.SolarWm2 = nf(lux), nf(uv), nf(solar)
	o.RainMm, o.StrikeDistKm, o.StrikeCount, o.BatteryV = nf(rain), nf(sdist), nf(scount), nf(batt)
	return o, nil
}

// Coverage summarizes how much data the archive holds.
type Coverage struct {
	Count    int64
	MinEpoch sql.NullInt64
	MaxEpoch sql.NullInt64
}

func (s *Store) archiveRange(ctx context.Context) (start, end int64, ok bool, err error) {
	coverage, err := s.Coverage(ctx)
	if err != nil {
		return 0, 0, false, err
	}
	if !coverage.MinEpoch.Valid || !coverage.MaxEpoch.Valid {
		return 0, 0, false, nil
	}
	if err := validateEpochRange(coverage.MinEpoch.Int64, coverage.MaxEpoch.Int64); err != nil {
		return 0, 0, false, fmt.Errorf("archive coverage: %w", err)
	}
	return coverage.MinEpoch.Int64, coverage.MaxEpoch.Int64, true, nil
}

// Coverage returns the row count and epoch range of the archive.
func (s *Store) Coverage(ctx context.Context) (Coverage, error) {
	var c Coverage
	db, err := s.database()
	if err != nil {
		return c, err
	}
	err = db.QueryRowContext(ctx, qryCoverage).
		Scan(&c.Count, &c.MinEpoch, &c.MaxEpoch)
	return c, archiveFailure("read archive coverage", err)
}

// Gap is a stretch of missing data between two consecutive stored observations
// whose separation exceeds the query threshold: the archive holds nothing
// between From and To.
type Gap struct {
	From      int64  `json:"from_epoch"`
	To        int64  `json:"to_epoch"`
	Seconds   int64  `json:"gap_seconds"`
	FromLocal string `json:"from"`
	ToLocal   string `json:"to"`
}

// Gaps returns up to limit coverage gaps (pairs of consecutive observations
// more than minSeconds apart), largest first. It's how archive_status shows an
// agent where history is missing so a targeted backfill can fill it. An
// archive with no qualifying gap returns an empty slice.
func (s *Store) Gaps(ctx context.Context, minSeconds int64, limit int) (_ []Gap, err error) {
	if minSeconds <= 0 {
		return nil, fmt.Errorf("%w: minimum gap must be positive", ErrInvalidArgument)
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > MaxGapResults {
		return nil, fmt.Errorf("%w: gap limit exceeds %d", ErrInvalidArgument, MaxGapResults)
	}
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, qryGaps, minSeconds, limit)
	if err != nil {
		return nil, archiveFailure("read archive gaps", err)
	}
	defer func() { err = errors.Join(err, archiveFailure("close archive gaps", rows.Close())) }()
	var out []Gap
	for rows.Next() {
		var g Gap
		if err := rows.Scan(&g.From, &g.To, &g.Seconds); err != nil {
			return nil, archiveFailure("scan archive gap", err)
		}
		g.FromLocal = localTime(g.From)
		g.ToLocal = localTime(g.To)
		out = append(out, g)
	}
	return out, archiveFailure("read archive gaps", rows.Err())
}

// Latest returns the most recent observation, or (nil, nil) if the archive is
// empty.
func (s *Store) Latest(ctx context.Context) (_ *model.Obs, err error) {
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, qryLatest)
	if err != nil {
		return nil, archiveFailure("read latest observation", err)
	}
	defer func() { err = errors.Join(err, archiveFailure("close latest observation", rows.Close())) }()
	if !rows.Next() {
		return nil, archiveFailure("read latest observation", rows.Err())
	}
	o, err := scanObs(rows)
	if err != nil {
		return nil, archiveFailure("scan latest observation", err)
	}
	return &o, nil
}

// EachObs calls fn for every stored observation in [startEpoch, endEpoch],
// oldest first, streaming one row at a time so an export of the whole archive
// never materializes every observation in memory at once. An error from fn
// stops the scan and is returned.
func (s *Store) EachObs(ctx context.Context, startEpoch, endEpoch int64, fn func(model.Obs) error) (err error) {
	if err := validateEpochRange(startEpoch, endEpoch); err != nil {
		return err
	}
	if fn == nil {
		return fmt.Errorf("%w: observation callback is nil", ErrInvalidArgument)
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, qryRange, startEpoch, endEpoch)
	if err != nil {
		return archiveFailure("read observations", err)
	}
	defer func() { err = errors.Join(err, archiveFailure("close observations", rows.Close())) }()
	for rows.Next() {
		o, err := scanObs(rows)
		if err != nil {
			return archiveFailure("scan observation", err)
		}
		if err := fn(o); err != nil {
			return err
		}
	}
	return archiveFailure("read observations", rows.Err())
}

// PressureTrend is the barometric tendency over a trailing window: the change
// between the latest station-pressure reading and the one nearest the window's
// start, with a plain-language category. Pressures are in inches of mercury (the
// US barometer unit); the raw millibar change drives the category.
type PressureTrend struct {
	At            int64   `json:"at_epoch"`
	Time          string  `json:"time"`
	CurrentInHg   float64 `json:"current_inhg"`
	WindowHours   float64 `json:"window_hours"`     // actual span between the two samples
	ChangeInHg    float64 `json:"change_inhg"`      // current minus the older sample
	ChangeMbPer3h float64 `json:"change_mb_per_3h"` // the standardized 3-hour rate
	Category      string  `json:"category"`         // rising/falling (rapidly), or steady
}

// pressureTendency classifies a barometric rate in millibars per 3 hours into
// the marine-barometer words. The 0.5 / 2.0 mb/3h cutoffs are the common
// steady/normal/rapid boundaries.
func pressureTendency(mbPer3h float64) string {
	switch {
	case mbPer3h >= 2.0:
		return "rising rapidly"
	case mbPer3h >= 0.5:
		return "rising"
	case mbPer3h <= -2.0:
		return "falling rapidly"
	case mbPer3h <= -0.5:
		return "falling"
	default:
		return "steady"
	}
}

// PressureTendency measures the change in station pressure over the trailing
// windowSeconds (3 hours is the meteorological standard). It reads the newest
// pressure and the reading nearest the window's start. It reports false when the
// archive lacks a pressure reading, or when the two samples are closer than half
// the window (too little history for a meaningful trend). The reported window is
// the actual span between the samples, so a sparse archive is not misrepresented.
func (s *Store) PressureTendency(ctx context.Context, windowSeconds int64) (*PressureTrend, bool, error) {
	if windowSeconds <= 0 {
		windowSeconds = 3 * 3600
	}
	if windowSeconds > 24*3600 {
		return nil, false, fmt.Errorf("%w: pressure window exceeds 24 hours", ErrInvalidArgument)
	}
	db, err := s.database()
	if err != nil {
		return nil, false, archiveFailure("read current pressure", err)
	}
	var nowEpoch int64
	var nowMb float64
	err = db.QueryRowContext(ctx, qryPressureAt, maxEpoch).Scan(&nowEpoch, &nowMb)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil // no pressure in the archive
	}
	if err != nil {
		return nil, false, archiveFailure("read prior pressure", err)
	}

	var pastEpoch int64
	var pastMb float64
	err = db.QueryRowContext(ctx, qryPressureAt, nowEpoch-windowSeconds).Scan(&pastEpoch, &pastMb)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	span := nowEpoch - pastEpoch
	if span < windowSeconds/2 {
		// The older sample is too recent (a short archive, or a long gap right
		// before now): not enough separation to call a trend.
		return nil, false, nil
	}

	changeMb := nowMb - pastMb
	hours := float64(span) / 3600
	pt := &PressureTrend{
		At: nowEpoch, Time: localTime(nowEpoch),
		CurrentInHg:   model.MbToInHg(nowMb),
		WindowHours:   hours,
		ChangeInHg:    model.MbToInHg(changeMb),
		ChangeMbPer3h: changeMb / hours * 3,
	}
	pt.Category = pressureTendency(pt.ChangeMbPer3h)
	return pt, true, nil
}

// DayStat is one calendar day of aggregates, in US display units.
type DayStat struct {
	Day         string   `json:"day"` // YYYY-MM-DD, local time
	TempMinF    *float64 `json:"temp_min_f,omitempty"`
	TempMaxF    *float64 `json:"temp_max_f,omitempty"`
	TempAvgF    *float64 `json:"temp_avg_f,omitempty"`
	RainIn      float64  `json:"rain_in"`
	PeakGustMph *float64 `json:"peak_gust_mph,omitempty"`
	Obs         int64    `json:"obs"`
}

// dayAgg is one local calendar day of raw SI aggregates: the intermediate the
// calendar-bucketed queries (DailySummary, PeriodSummary, wettest day) roll up
// from. tempSum/tempN carry the exact mean; pointer fields are nil when the day
// had no reading for them.
type dayAgg struct {
	day     time.Time // local midnight
	tempMin *float64
	tempMax *float64
	tempSum float64
	tempN   int64
	rainMm  float64
	gustMax *float64
	obs     int64
}

// rollupBucketSeconds is the pre-aggregation bucket for calendar queries: 15
// minutes. Every real-world UTC offset (and DST shift) is a multiple of 15
// minutes, so a bucket never straddles a local calendar-day boundary, which
// makes it safe to aggregate buckets with pure integer SQL and assign each
// whole bucket to a local day in Go (see day_rollup.sql).
const rollupBucketSeconds = 900

// dayAggregates rolls the observations in [startEpoch, endEpoch] up to local
// calendar days, oldest first: one integer-bucketed scan in SQL, then a cheap
// bucket→day merge in Go (see rollupBucketSeconds for why this is exact).
func (s *Store) dayAggregates(ctx context.Context, startEpoch, endEpoch int64) (_ []dayAgg, err error) {
	if err := validateEpochRange(startEpoch, endEpoch); err != nil {
		return nil, err
	}
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, qryDayRollup, rollupBucketSeconds, startEpoch, endEpoch)
	if err != nil {
		return nil, archiveFailure("read daily aggregates", err)
	}
	defer func() { err = errors.Join(err, archiveFailure("close daily aggregates", rows.Close())) }()

	var out []dayAgg
	for rows.Next() {
		var b, tempN, obs int64
		var tmin, tmax, gust sql.NullFloat64
		var tempSum, rainMm float64
		if err := rows.Scan(&b, &tmin, &tmax, &tempSum, &tempN, &rainMm, &gust, &obs); err != nil {
			return nil, archiveFailure("scan daily aggregate", err)
		}
		t := time.Unix(b*rollupBucketSeconds, 0).Local()
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
		if len(out) == 0 || !out[len(out)-1].day.Equal(day) {
			out = append(out, dayAgg{day: day})
		}
		cur := &out[len(out)-1]
		cur.tempMin = minPtr(cur.tempMin, nf(tmin))
		cur.tempMax = maxPtr(cur.tempMax, nf(tmax))
		cur.tempSum += tempSum
		cur.tempN += tempN
		cur.rainMm += rainMm
		cur.gustMax = maxPtr(cur.gustMax, nf(gust))
		cur.obs += obs
	}
	return out, archiveFailure("read daily aggregates", rows.Err())
}

// dayStat converts a raw SI day aggregate to display units.
func dayStat(d dayAgg) DayStat {
	ds := DayStat{Day: d.day.Format("2006-01-02"), RainIn: model.MmToInch(d.rainMm), Obs: d.obs}
	if d.tempMin != nil {
		v := model.CToF(*d.tempMin)
		ds.TempMinF = &v
	}
	if d.tempMax != nil {
		v := model.CToF(*d.tempMax)
		ds.TempMaxF = &v
	}
	if d.tempN > 0 {
		v := model.CToF(d.tempSum / float64(d.tempN))
		ds.TempAvgF = &v
	}
	if d.gustMax != nil {
		v := model.MpsToMph(*d.gustMax)
		ds.PeakGustMph = &v
	}
	return ds
}

// DailySummary aggregates observations by local calendar day within the
// inclusive epoch range [startEpoch, endEpoch].
func (s *Store) DailySummary(ctx context.Context, startEpoch, endEpoch int64) ([]DayStat, error) {
	if err := validateEpochRange(startEpoch, endEpoch); err != nil {
		return nil, err
	}
	if _, err := s.database(); err != nil {
		return nil, err
	}
	days, err := s.dayAggregates(ctx, startEpoch, endEpoch)
	if err != nil {
		return nil, err
	}
	out := make([]DayStat, 0, len(days))
	for _, d := range days {
		out = append(out, dayStat(d))
	}
	return out, nil
}

// HourStat is one local hour-of-day of climatological averages over the queried
// window, in US display units: the "typical" conditions at that hour.
type HourStat struct {
	Hour        int      `json:"hour"` // 0-23, local time
	TempAvgF    *float64 `json:"temp_avg_f,omitempty"`
	TempMinF    *float64 `json:"temp_min_f,omitempty"`
	TempMaxF    *float64 `json:"temp_max_f,omitempty"`
	HumidityPct *float64 `json:"humidity_pct,omitempty"`
	WindMph     *float64 `json:"wind_mph,omitempty"`
	PeakGustMph *float64 `json:"peak_gust_mph,omitempty"`
	Obs         int64    `json:"obs"`
}

// hourAcc accumulates 15-minute buckets into one local hour-of-day slot. Temp,
// humidity, and wind keep running sums for a true mean over every observation;
// min/max and peak gust keep the extreme seen at that hour across all days.
type hourAcc struct {
	tempMin, tempMax *float64
	tempSum          float64
	tempN            int64
	humSum           float64
	humN             int64
	windSum          float64
	windN            int64
	gustMax          *float64
	obs              int64
}

// HourlyClimatology folds the observations in [startEpoch, endEpoch] into the 24
// local hours of the day, oldest hour first, reporting the mean temperature,
// humidity, and wind at each hour plus the min/max temperature and peak gust
// ever seen in that hour. It answers "when is it typically coldest / windiest?"
// Hours the archive never observed are omitted. Like the calendar rollups it
// buckets to 15 minutes in SQL (which never straddles a local hour boundary) and
// assigns each bucket to its hour in Go.
func (s *Store) HourlyClimatology(ctx context.Context, startEpoch, endEpoch int64) (_ []HourStat, err error) {
	if err := validateEpochRange(startEpoch, endEpoch); err != nil {
		return nil, err
	}
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, qryHourRollup, rollupBucketSeconds, startEpoch, endEpoch)
	if err != nil {
		return nil, archiveFailure("read hourly climatology", err)
	}
	defer func() { err = errors.Join(err, archiveFailure("close hourly climatology", rows.Close())) }()

	var acc [24]hourAcc
	for rows.Next() {
		var b, tempN, humN, windN, obs int64
		var tmin, tmax, gust sql.NullFloat64
		var tempSum, humSum, windSum float64
		if err := rows.Scan(&b, &tmin, &tmax, &tempSum, &tempN,
			&humSum, &humN, &windSum, &windN, &gust, &obs); err != nil {
			return nil, archiveFailure("scan hourly climatology", err)
		}
		h := time.Unix(b*rollupBucketSeconds, 0).Local().Hour()
		a := &acc[h]
		a.tempMin = minPtr(a.tempMin, nf(tmin))
		a.tempMax = maxPtr(a.tempMax, nf(tmax))
		a.tempSum += tempSum
		a.tempN += tempN
		a.humSum += humSum
		a.humN += humN
		a.windSum += windSum
		a.windN += windN
		a.gustMax = maxPtr(a.gustMax, nf(gust))
		a.obs += obs
	}
	if err := rows.Err(); err != nil {
		return nil, archiveFailure("read hourly climatology", err)
	}

	var out []HourStat
	for h := range 24 {
		a := acc[h]
		if a.obs == 0 {
			continue
		}
		hs := HourStat{Hour: h, Obs: a.obs}
		hs.TempMinF, hs.TempMaxF = cPtrToF(a.tempMin), cPtrToF(a.tempMax)
		if a.tempN > 0 {
			v := model.CToF(a.tempSum / float64(a.tempN))
			hs.TempAvgF = &v
		}
		if a.humN > 0 {
			v := a.humSum / float64(a.humN)
			hs.HumidityPct = &v
		}
		if a.windN > 0 {
			v := model.MpsToMph(a.windSum / float64(a.windN))
			hs.WindMph = &v
		}
		if a.gustMax != nil {
			v := model.MpsToMph(*a.gustMax)
			hs.PeakGustMph = &v
		}
		out = append(out, hs)
	}
	return out, nil
}

// cPtrToF converts a nullable Celsius pointer to a °F pointer (nil stays nil).
func cPtrToF(c *float64) *float64 {
	if c == nil {
		return nil
	}
	v := model.CToF(*c)
	return &v
}

// minPtr and maxPtr merge nullable readings, treating nil as "no reading yet".
func minPtr(a, b *float64) *float64 {
	if a == nil || (b != nil && *b < *a) {
		return b
	}
	return a
}

func maxPtr(a, b *float64) *float64 {
	if a == nil || (b != nil && *b > *a) {
		return b
	}
	return a
}

// Records holds all-time extremes across the whole archive.
type Records struct {
	HottestF      *float64 `json:"hottest_f,omitempty"`
	HottestEpoch  *int64   `json:"hottest_epoch,omitempty"`
	ColdestF      *float64 `json:"coldest_f,omitempty"`
	ColdestEpoch  *int64   `json:"coldest_epoch,omitempty"`
	PeakGustMph   *float64 `json:"peak_gust_mph,omitempty"`
	PeakGustEpoch *int64   `json:"peak_gust_epoch,omitempty"`
	WettestDay    string   `json:"wettest_day,omitempty"`
	WettestDayIn  *float64 `json:"wettest_day_in,omitempty"`
	TotalStrikes  *float64 `json:"total_lightning_strikes,omitempty"`

	LowestPressureInHg  *float64 `json:"lowest_pressure_inhg,omitempty"`
	LowestPressureEpoch *int64   `json:"lowest_pressure_epoch,omitempty"`
	PeakSolarWm2        *float64 `json:"peak_solar_wm2,omitempty"`
	PeakSolarEpoch      *int64   `json:"peak_solar_epoch,omitempty"`
	PeakUV              *float64 `json:"peak_uv,omitempty"`
	PeakUVEpoch         *int64   `json:"peak_uv_epoch,omitempty"`
}

// epochOf finds the first epoch whose column equals the extreme value found by
// the single-pass scan in Records. col is an internal constant (never user
// input) substituted into epoch_of.sql via template; see queries.go.
func (s *Store) epochOf(ctx context.Context, col string, v float64) (*int64, error) {
	db, err := s.database()
	if err != nil {
		return nil, archiveFailure("read record epoch", err)
	}
	var e int64
	err = db.QueryRowContext(ctx, epochOfSQL(col), v).Scan(&e)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// Records computes all-time extremes: one full scan gathers every scalar
// extreme at once, cheap equality lookups recover their timestamps, and the
// wettest day comes from the shared calendar rollup.
func (s *Store) Records(ctx context.Context) (Records, error) {
	var r Records
	db, err := s.database()
	if err != nil {
		return r, err
	}

	var hot, cold, gust, strikes, lowP, solar, uv sql.NullFloat64
	if err := db.QueryRowContext(ctx, qryRecordsExtremes).
		Scan(&hot, &cold, &gust, &strikes, &lowP, &solar, &uv); err != nil {
		return r, archiveFailure("read archive records", err)
	}
	if hot.Valid {
		f := model.CToF(hot.Float64)
		e, err := s.epochOf(ctx, "air_temp_c", hot.Float64)
		if err != nil {
			return r, err
		}
		r.HottestF, r.HottestEpoch = &f, e
	}
	if cold.Valid {
		f := model.CToF(cold.Float64)
		e, err := s.epochOf(ctx, "air_temp_c", cold.Float64)
		if err != nil {
			return r, err
		}
		r.ColdestF, r.ColdestEpoch = &f, e
	}
	if gust.Valid {
		mph := model.MpsToMph(gust.Float64)
		e, err := s.epochOf(ctx, "wind_gust", gust.Float64)
		if err != nil {
			return r, err
		}
		r.PeakGustMph, r.PeakGustEpoch = &mph, e
	}
	if strikes.Valid {
		r.TotalStrikes = &strikes.Float64
	}
	if lowP.Valid {
		inhg := model.MbToInHg(lowP.Float64)
		e, err := s.epochOf(ctx, "pressure_mb", lowP.Float64)
		if err != nil {
			return r, err
		}
		r.LowestPressureInHg, r.LowestPressureEpoch = &inhg, e
	}
	if solar.Valid {
		v := solar.Float64
		e, err := s.epochOf(ctx, "solar_wm2", v)
		if err != nil {
			return r, err
		}
		r.PeakSolarWm2, r.PeakSolarEpoch = &v, e
	}
	if uv.Valid {
		v := uv.Float64
		e, err := s.epochOf(ctx, "uv", v)
		if err != nil {
			return r, err
		}
		r.PeakUV, r.PeakUVEpoch = &v, e
	}

	start, end, ok, err := s.archiveRange(ctx)
	if err != nil || !ok {
		return r, err
	}
	days, err := s.dayAggregates(ctx, start, end)
	if err != nil {
		return r, err
	}
	var bestMm float64
	for _, d := range days {
		if d.rainMm > bestMm {
			bestMm = d.rainMm
			r.WettestDay = d.day.Format("2006-01-02")
		}
	}
	if bestMm > 0 {
		in := model.MmToInch(bestMm)
		r.WettestDayIn = &in
	}
	return r, nil
}

// maxEpoch is an epoch upper bound safely beyond any real observation
// (year ~4147) while leaving integer headroom for bucket arithmetic.
const maxEpoch = int64(1) << 36

// localTime renders an epoch as a local RFC3339 timestamp for human-facing gap
// reports.
func localTime(epoch int64) string {
	return time.Unix(epoch, 0).Local().Format(time.RFC3339)
}
