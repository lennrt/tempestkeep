// Package collect appends Tempest history to a local archive. It fetches at most
// api.MaxDeviceWindow per request. Committed chunks are replay-safe. Result.Resume
// identifies the next range after an interrupted operation.
package collect

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

// Meta keys shared by every collector front end (the CLI and the MCP
// backfill_archive tool), so a walk-back started by one resumes in the other.
const (
	// MetaBackfillCursor stores the epoch an open-ended walk-back has reached,
	// persisted after every chunk. Without it an interrupted seed looks finished
	// on the next run (a watermark exists, so the collector switches to forward
	// sync) and the older history below the interruption is never fetched.
	MetaBackfillCursor = "backfill_cursor"
	// MetaBackfillComplete marks that a walk-back reached the start of history,
	// so later runs skip the walk instead of re-probing empty windows.
	MetaBackfillComplete = "backfill_complete"
)

// ObsFetcher fetches observations for an inclusive epoch-second range.
type ObsFetcher interface {
	DeviceObservations(ctx context.Context, deviceID int, start, end int64) ([]model.DeviceObs, error)
}

// DefaultEmptyStreakStop ends an open backward walk after three empty windows.
// With the default chunk, this is 15 days. Pass a floor to BackfillBackward when
// the exact lower bound is known.
const (
	DefaultEmptyStreakStop = 3
	MaxChunksPerOperation  = 10_000
	MaxEmptyStreakStop     = 100
	MaxThrottle            = time.Minute
)

var (
	ErrInvalidConfig     = errors.New("invalid collector configuration")
	ErrConcurrentUse     = errors.New("collector is already in use")
	ErrInvalidCheckpoint = errors.New("invalid collector checkpoint")
	ErrOperationLimit    = errors.New("collector operation limit reached")
)

type backfillerConfig struct {
	chunkSeconds int64
	throttle     time.Duration
	emptyStreak  int
	progress     ProgressFunc
}

// Option configures a Backfiller without performing I/O.
type Option func(*backfillerConfig) error

// WithChunkSeconds sets the API window. The value must be from 1 second through
// the WeatherFlow five-day limit.
func WithChunkSeconds(seconds int64) Option {
	return func(config *backfillerConfig) error {
		maximum := int64(api.MaxDeviceWindow / time.Second)
		if seconds <= 0 || seconds > maximum {
			return fmt.Errorf("%w: chunk seconds must be in 1..%d", ErrInvalidConfig, maximum)
		}
		config.chunkSeconds = seconds
		return nil
	}
}

// WithThrottle sets the delay between API requests. Zero disables the delay.
func WithThrottle(delay time.Duration) Option {
	return func(config *backfillerConfig) error {
		if delay < 0 || delay > MaxThrottle {
			return fmt.Errorf("%w: throttle must be between 0 and %s", ErrInvalidConfig, MaxThrottle)
		}
		config.throttle = delay
		return nil
	}
}

// WithEmptyStreakStop sets the empty-window limit for an open-ended walk.
func WithEmptyStreakStop(windows int) Option {
	return func(config *backfillerConfig) error {
		if windows < 1 || windows > MaxEmptyStreakStop {
			return fmt.Errorf("%w: empty streak must be in 1..%d", ErrInvalidConfig, MaxEmptyStreakStop)
		}
		config.emptyStreak = windows
		return nil
	}
}

// WithProgress sets a borrowed callback. The caller must keep it valid for the
// Backfiller lifetime. An error stops the operation after the current commit.
func WithProgress(progress ProgressFunc) Option {
	return func(config *backfillerConfig) error {
		if progress == nil {
			return fmt.Errorf("%w: progress callback is nil", ErrInvalidConfig)
		}
		config.progress = progress
		return nil
	}
}

// Backfiller appends one device's history in bounded chunks. It borrows the
// fetcher, writer, and progress callback. One operation can run at a time.
type Backfiller struct {
	fetcher     ObsFetcher
	writer      *store.Writer
	deviceID    int
	chunk       int64
	throttle    time.Duration
	emptyStreak int
	progress    ProgressFunc
	inUse       atomic.Bool
}

// New validates dependencies and options. It does not access the network,
// filesystem, archive, environment, or start a goroutine.
func New(fetcher ObsFetcher, writer *store.Writer, deviceID int, options ...Option) (*Backfiller, error) {
	if nilInterface(fetcher) || writer == nil || deviceID <= 0 {
		return nil, fmt.Errorf("%w: fetcher and writer are required, and device id must be positive", ErrInvalidConfig)
	}
	config := backfillerConfig{
		chunkSeconds: int64(api.MaxDeviceWindow / time.Second),
		emptyStreak:  DefaultEmptyStreakStop,
	}
	for index, option := range options {
		if option == nil {
			return nil, fmt.Errorf("%w: option %d is nil", ErrInvalidConfig, index)
		}
		if err := option(&config); err != nil {
			return nil, fmt.Errorf("option %d: %w", index, err)
		}
	}
	return &Backfiller{
		fetcher: fetcher, writer: writer, deviceID: deviceID,
		chunk: config.chunkSeconds, throttle: config.throttle,
		emptyStreak: config.emptyStreak, progress: config.progress,
	}, nil
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() { //nolint:exhaustive // Only kinds that can be nil need explicit handling.
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// Progress reports one committed chunk of a walk. Through is the epoch the walk
// has covered to: the newest fetched second for a forward walk, the oldest for a
// backward one.
type Progress struct {
	Fetched   int
	RowsAdded int
	Chunks    int
	Through   int64
}

// ProgressFunc receives one committed progress update.
type ProgressFunc func(Progress) error

func (b *Backfiller) reportProgress(progress ProgressFunc, fetched, rows, chunks int, through int64) error {
	if progress != nil {
		return progress(Progress{Fetched: fetched, RowsAdded: rows, Chunks: chunks, Through: through})
	}
	return nil
}

func (b *Backfiller) begin() error {
	if b == nil || b.fetcher == nil || b.writer == nil || b.deviceID <= 0 {
		return fmt.Errorf("%w: collector is nil or not initialized", ErrInvalidConfig)
	}
	if !b.inUse.CompareAndSwap(false, true) {
		return ErrConcurrentUse
	}
	return nil
}

func (b *Backfiller) finish() {
	b.inUse.Store(false)
}

func chunkBudget(maxChunks int) (int, error) {
	if maxChunks < 0 || maxChunks > MaxChunksPerOperation {
		return 0, fmt.Errorf("%w: max chunks must be in 0..%d", ErrInvalidConfig, MaxChunksPerOperation)
	}
	if maxChunks == 0 {
		return MaxChunksPerOperation, nil
	}
	return maxChunks, nil
}

func validateEpochRange(start, end int64) error {
	if start < 0 || end < 0 || start > end || end > model.MaxEpochSeconds {
		return fmt.Errorf("%w: epoch range must be ordered and within 0..%d", ErrInvalidConfig, model.MaxEpochSeconds)
	}
	return nil
}

// Result summarizes a forward backfill (BackfillRange) or a Sync.
type Result struct {
	Fetched   int   // observations the API returned across all chunks
	RowsAdded int   // rows newly inserted (duplicates ignored, not counted)
	Resume    int64 // epoch to resume from: end+1 when the range completed, else the failed chunk's start
	Done      bool  // true when the whole requested range was covered
	// NoWatermark is set only by Sync, when the device has no stored data yet, so
	// the caller can seed the archive with a backfill before syncing.
	NoWatermark bool
}

// BackfillRange fetches the inclusive epoch-seconds range [start, end] into the
// archive, walking forward in chunks no wider than the configured window and
// appending each. maxChunks > 0 bounds the windows requested in this call, so a
// tool call over a months-stale archive stays inside client timeouts; the walk
// stops with Done false and Resume set for the next call. On the first fetch or
// store error it stops and returns the error with Resume set to the failed
// chunk's start (rows from earlier chunks are already committed); a later call
// from Resume finishes the job. Because writes are INSERT OR IGNORE, re-running
// over any range is safe.
func (b *Backfiller) BackfillRange(ctx context.Context, start, end int64, maxChunks int) (Result, error) {
	if err := b.begin(); err != nil {
		return Result{Resume: start}, err
	}
	defer b.finish()
	budget, err := chunkBudget(maxChunks)
	if err != nil {
		return Result{Resume: start}, err
	}
	return b.backfillRange(ctx, start, end, budget, b.progress)
}

func (b *Backfiller) backfillRange(ctx context.Context, start, end int64, maxChunks int, progress ProgressFunc) (Result, error) {
	res := Result{Resume: start}
	if err := validateEpochRange(start, end); err != nil {
		return res, err
	}
	if err := b.writer.BindDevice(ctx, b.deviceID); err != nil {
		return res, err
	}
	chunks := 0
	for s := start; s <= end; {
		e := min(s+b.chunk-1, end)
		obs, err := b.fetcher.DeviceObservations(ctx, b.deviceID, s, e)
		if err != nil {
			res.Resume = s
			return res, fmt.Errorf("fetch observation chunk: %w", err)
		}
		added, err := b.writer.InsertObs(ctx, b.deviceID, obs)
		if err != nil {
			res.Resume = s
			return res, fmt.Errorf("store observation chunk: %w", err)
		}
		res.Fetched += len(obs)
		res.RowsAdded += added

		s = e + 1
		res.Resume = s
		chunks++
		if err := b.reportProgress(progress, res.Fetched, res.RowsAdded, chunks, e); err != nil {
			return res, fmt.Errorf("report committed chunk: %w", err)
		}
		if chunks >= maxChunks && s <= end {
			return res, nil // budget spent; caller resumes from Resume
		}
		if s <= end && b.throttle > 0 {
			if err := sleep(ctx, b.throttle); err != nil {
				return res, err
			}
		}
	}
	res.Done = true
	return res, nil
}

// BackwardResult summarizes a bounded backward walk.
type BackwardResult struct {
	Fetched   int   // observations the API returned across the walk
	RowsAdded int   // rows newly inserted
	Chunks    int   // chunk windows actually requested
	Reached   int64 // oldest epoch fetched to; pass as `before` to continue an unexhausted walk
	Exhausted bool  // true once history ended (empty streak) or the floor was reached; nothing older to fetch
}

// BackfillBackward walks history backward from before (exclusive: it fetches
// windows strictly older than before), appending each chunk. It stops when any
// of these holds:
//
//   - it has requested maxChunks windows (maxChunks > 0): a bounded batch, so an
//     agent tool or CLI stays responsive and resumes from Reached next call;
//   - a window reaches or passes floor (floor > 0): the exact oldest epoch wanted;
//   - EmptyStreakStop windows in a row come back empty: history has ended.
//
// The last two set Exhausted (no older data remains); the first does not. Because
// every write is INSERT OR IGNORE, the walk is idempotent and fully resumable:
// re-running from Reached adds only what's missing.
func (b *Backfiller) BackfillBackward(ctx context.Context, before, floor int64, maxChunks int) (BackwardResult, error) {
	if err := b.begin(); err != nil {
		return BackwardResult{Reached: before}, err
	}
	defer b.finish()
	budget, err := chunkBudget(maxChunks)
	if err != nil {
		return BackwardResult{Reached: before}, err
	}
	return b.backfillBackward(ctx, before, floor, budget, b.progress)
}

func (b *Backfiller) backfillBackward(ctx context.Context, before, floor int64, maxChunks int, progress ProgressFunc) (BackwardResult, error) {
	res := BackwardResult{Reached: before}
	if before <= 0 || before > model.MaxEpochSeconds || floor < 0 || floor >= before {
		return res, fmt.Errorf("%w: before must be in 1..%d and floor must be in 0..before-1", ErrInvalidConfig, model.MaxEpochSeconds)
	}
	if err := b.writer.BindDevice(ctx, b.deviceID); err != nil {
		return res, err
	}
	emptyStreak := 0

	for end := before - 1; end >= floor; {
		start := max(int64(0), end-b.chunk+1)
		if floor > 0 && start < floor {
			start = floor
		}
		obs, err := b.fetcher.DeviceObservations(ctx, b.deviceID, start, end)
		if err != nil {
			return res, fmt.Errorf("fetch observation chunk: %w", err)
		}
		added, err := b.writer.InsertObs(ctx, b.deviceID, obs)
		if err != nil {
			return res, fmt.Errorf("store observation chunk: %w", err)
		}
		res.Fetched += len(obs)
		res.RowsAdded += added
		res.Chunks++
		res.Reached = start
		if err := b.reportProgress(progress, res.Fetched, res.RowsAdded, res.Chunks, start); err != nil {
			return res, fmt.Errorf("report committed chunk: %w", err)
		}

		if len(obs) == 0 {
			emptyStreak++
		} else {
			emptyStreak = 0
		}
		if start == 0 || floor > 0 && start <= floor {
			res.Exhausted = true
			return res, nil
		}
		// The streak heuristic only applies to open-ended walks. With an
		// explicit floor the caller has asserted how far back to go, and an
		// outage longer than stop*chunk mid-history must not end the walk
		// early and falsely report history exhausted.
		if floor == 0 && emptyStreak >= b.emptyStreak {
			res.Exhausted = true
			return res, nil
		}
		if res.Chunks >= maxChunks {
			return res, nil // bounded batch done; caller resumes from Reached
		}

		end = start - 1
		if b.throttle > 0 {
			if err := sleep(ctx, b.throttle); err != nil {
				return res, err
			}
		}
	}
	res.Exhausted = true
	return res, nil
}

// Sync brings the archive current: it forward-fills from the newest stored
// observation for the device up to now, bounded by maxChunks like
// BackfillRange. Zero uses the hard operation limit. It reports NoWatermark (with Done true and
// nothing fetched) when the device has no data yet, so the caller can seed the
// archive with a backfill first. Cheap to run often: it fetches only what's
// newer than the watermark.
func (b *Backfiller) Sync(ctx context.Context, now int64, maxChunks int) (Result, error) {
	if err := b.begin(); err != nil {
		return Result{}, err
	}
	defer b.finish()
	budget, err := chunkBudget(maxChunks)
	if err != nil {
		return Result{}, err
	}
	return b.sync(ctx, now, budget)
}

func (b *Backfiller) sync(ctx context.Context, now int64, maxChunks int) (Result, error) {
	if now <= 0 || now > model.MaxEpochSeconds {
		return Result{}, fmt.Errorf("%w: current epoch must be in 1..%d", ErrInvalidConfig, model.MaxEpochSeconds)
	}
	if err := b.writer.BindDevice(ctx, b.deviceID); err != nil {
		return Result{}, err
	}
	wm, ok, err := b.writer.Watermark(ctx, b.deviceID)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{Done: true, NoWatermark: true, Resume: now}, nil
	}
	if wm+1 > now {
		return Result{Done: true, Resume: now + 1}, nil // already current
	}
	return b.backfillRange(ctx, wm+1, now, maxChunks, b.progress)
}

// Summary summarizes a one-call Collect run for CLI reporting.
type Summary struct {
	Mode      string // "incremental", "backfill", "seed", or "incremental+seed"
	Fetched   int
	RowsAdded int
}

// Collect performs the whole-device collector behavior,
// exposed as one call for `tempest collect`:
//
//   - if the device already has data, incrementally Sync forward to now, then
//     resume an interrupted seed walk if one left a cursor behind;
//   - else if backfillStart > 0, forward-fill from that epoch to now;
//   - else seed the archive by walking history all the way back until it ends.
//
// It is safe to re-run any time (idempotent writes), including after Ctrl-C:
// the seed walk persists its cursor per chunk, and a forward fill resumes from
// the watermark.
func (b *Backfiller) Collect(ctx context.Context, now, backfillStart int64) (Summary, error) {
	if err := b.begin(); err != nil {
		return Summary{}, err
	}
	defer b.finish()
	if now <= 0 || now > model.MaxEpochSeconds || backfillStart < 0 || backfillStart > now {
		return Summary{}, fmt.Errorf("%w: collection range is outside 0..%d or is not ordered", ErrInvalidConfig, model.MaxEpochSeconds)
	}
	if err := b.writer.BindDevice(ctx, b.deviceID); err != nil {
		return Summary{}, err
	}
	_, ok, err := b.writer.Watermark(ctx, b.deviceID)
	if err != nil {
		return Summary{}, err
	}
	if ok {
		r, err := b.sync(ctx, now, MaxChunksPerOperation)
		sum := Summary{Mode: "incremental", Fetched: r.Fetched, RowsAdded: r.RowsAdded}
		if err != nil {
			return sum, err
		}
		if !r.Done {
			return sum, fmt.Errorf("%w: resume forward collection", ErrOperationLimit)
		}
		before, resume, err := b.seedCursor(ctx)
		if err != nil || !resume {
			return sum, err
		}
		br, err := b.seedWalk(ctx, before)
		sum.Mode = "incremental+seed"
		sum.Fetched += br.Fetched
		sum.RowsAdded += br.RowsAdded
		return sum, err
	}
	if backfillStart > 0 {
		r, err := b.backfillRange(ctx, backfillStart, now, MaxChunksPerOperation, b.progress)
		if err == nil && !r.Done {
			err = fmt.Errorf("%w: resume forward collection", ErrOperationLimit)
		}
		return Summary{Mode: "backfill", Fetched: r.Fetched, RowsAdded: r.RowsAdded}, err
	}
	r, err := b.seedWalk(ctx, now) // walk back to the start of history
	return Summary{Mode: "seed", Fetched: r.Fetched, RowsAdded: r.RowsAdded}, err
}

// seedCursor reports whether an open-ended walk-back should resume, and from
// where. It resumes only when a prior seed left a cursor without the complete
// marker; an archive forward-filled from an explicit start has neither, and its
// deliberately excluded older history stays excluded.
func (b *Backfiller) seedCursor(ctx context.Context) (before int64, resume bool, err error) {
	if v, ok, err := b.writer.Meta(ctx, MetaBackfillComplete); err != nil {
		return 0, false, err
	} else if ok {
		switch v {
		case "1":
			return 0, false, nil
		case "0":
		default:
			return 0, false, fmt.Errorf("%w: %s must be 0 or 1", ErrInvalidCheckpoint, MetaBackfillComplete)
		}
	}
	v, ok, err := b.writer.Meta(ctx, MetaBackfillCursor)
	if err != nil || !ok {
		return 0, false, err
	}
	n, perr := strconv.ParseInt(v, 10, 64)
	if perr != nil || n <= 0 || n > model.MaxEpochSeconds {
		return 0, false, fmt.Errorf("%w: %s is not a valid epoch", ErrInvalidCheckpoint, MetaBackfillCursor)
	}
	return n, true, nil
}

// seedWalk runs an open-ended walk-back that persists its cursor after every
// chunk, so a Ctrl-C or crash mid-seed resumes on the next run. On exhaustion
// it sets the complete marker (shared with the MCP backfill_archive tool).
func (b *Backfiller) seedWalk(ctx context.Context, before int64) (BackwardResult, error) {
	progress := func(p Progress) error {
		checkpointErr := b.writer.SetMeta(ctx, MetaBackfillCursor, strconv.FormatInt(p.Through, 10))
		if b.progress == nil {
			return checkpointErr
		}
		return errors.Join(checkpointErr, b.progress(p))
	}

	r, err := b.backfillBackward(ctx, before, 0, MaxChunksPerOperation, progress)
	if err == nil && !r.Exhausted {
		err = fmt.Errorf("%w: resume backward collection", ErrOperationLimit)
	}
	if err == nil && r.Exhausted {
		err = b.writer.SetMeta(ctx, MetaBackfillComplete, "1")
	}
	return r, err
}

// sleep waits for d or until ctx is cancelled, whichever comes first.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
