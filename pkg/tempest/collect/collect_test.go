package collect_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/collect"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

const device = 456

// fakeFetcher records the windows requested and returns whatever obsFor yields,
// so the collector can be exercised without a network.
type fakeFetcher struct {
	calls  [][2]int64
	obsFor func(start, end int64) ([]model.DeviceObs, error)
}

func (f *fakeFetcher) DeviceObservations(_ context.Context, _ int, start, end int64) ([]model.DeviceObs, error) {
	f.calls = append(f.calls, [2]int64{start, end})
	return f.obsFor(start, end)
}

func newWriter(t *testing.T) *store.Writer {
	t.Helper()
	w, err := store.OpenWriter(t.Context(), filepath.Join(t.TempDir(), "archive.sqlite"))
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	closeOnCleanup(t, w)
	return w
}

func newBackfiller(t *testing.T, fetch collect.ObsFetcher, writer *store.Writer, options ...collect.Option) *collect.Backfiller {
	t.Helper()
	b, err := collect.New(fetch, writer, device, options...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return b
}

func TestBackfillRangeChunksAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	w := newWriter(t)
	// One observation per chunk, stamped at the window start.
	fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
		return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(20))}}, nil
	}}
	b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(100))

	res, err := b.BackfillRange(ctx, 1000, 1349, 0)
	if err != nil {
		t.Fatalf("BackfillRange: %v", err)
	}
	// The range is walked forward in contiguous, non-overlapping ≤100s windows.
	wantWindows := [][2]int64{{1000, 1099}, {1100, 1199}, {1200, 1299}, {1300, 1349}}
	if len(fetch.calls) != len(wantWindows) {
		t.Fatalf("fetch calls = %d, want %d (%v)", len(fetch.calls), len(wantWindows), fetch.calls)
	}
	for i, wnd := range wantWindows {
		if fetch.calls[i] != wnd {
			t.Errorf("window %d = %v, want %v", i, fetch.calls[i], wnd)
		}
	}
	if !res.Done || res.Resume != 1350 {
		t.Errorf("res = %+v, want Done with Resume 1350", res)
	}
	if res.RowsAdded != 4 || res.Fetched != 4 {
		t.Errorf("rows/fetched = %d/%d, want 4/4", res.RowsAdded, res.Fetched)
	}

	// Re-running the same range adds nothing, though it still fetches.
	res2, err := b.BackfillRange(ctx, 1000, 1349, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res2.RowsAdded != 0 {
		t.Errorf("re-run RowsAdded = %d, want 0 (idempotent)", res2.RowsAdded)
	}

	cov, err := w.Coverage(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	if cov.Count != 4 {
		t.Errorf("stored rows = %d, want 4", cov.Count)
	}
}

func TestBackfillRangeDefaultChunkAndThrottle(t *testing.T) {
	ctx := context.Background()
	w := newWriter(t)
	fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
		return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(20))}}, nil
	}}

	// ChunkSeconds 0 falls back to the 5-day default, so a 51-second range is a
	// single request. A small non-zero Throttle exercises the inter-chunk pause.
	b := newBackfiller(t, fetch, w, collect.WithThrottle(time.Millisecond))
	res, err := b.BackfillRange(ctx, 1000, 1050, 0)
	if err != nil {
		t.Fatalf("BackfillRange: %v", err)
	}
	if len(fetch.calls) != 1 || fetch.calls[0] != [2]int64{1000, 1050} {
		t.Errorf("calls = %v, want a single [1000,1050] window", fetch.calls)
	}
	if !res.Done || res.Resume != 1051 || res.RowsAdded != 1 {
		t.Errorf("res = %+v, want Done, Resume 1051, 1 row", res)
	}

	// With an explicit small chunk the same range needs two requests, so the
	// throttle actually fires between them.
	b = newBackfiller(t, fetch, w, collect.WithChunkSeconds(40), collect.WithThrottle(time.Millisecond))
	fetch.calls = nil
	res, err = b.BackfillRange(ctx, 2000, 2079, 0)
	if err != nil {
		t.Fatalf("BackfillRange (chunked): %v", err)
	}
	if len(fetch.calls) != 2 {
		t.Errorf("calls = %v, want 2 windows", fetch.calls)
	}
	if !res.Done || res.RowsAdded != 2 {
		t.Errorf("res = %+v, want Done with 2 rows", res)
	}
}

func TestBackfillBackwardBoundedBatch(t *testing.T) {
	ctx := context.Background()
	w := newWriter(t)
	fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
		return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(20))}}, nil
	}}
	b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(100))

	// maxChunks bounds the work: two windows below before=1000, then stop with
	// more history still available.
	res, err := b.BackfillBackward(ctx, 1000, 0, 2)
	if err != nil {
		t.Fatalf("BackfillBackward: %v", err)
	}
	wantWindows := [][2]int64{{900, 999}, {800, 899}}
	if len(fetch.calls) != 2 || fetch.calls[0] != wantWindows[0] || fetch.calls[1] != wantWindows[1] {
		t.Errorf("windows = %v, want %v", fetch.calls, wantWindows)
	}
	if res.Exhausted {
		t.Error("Exhausted = true, want false (maxChunks stop, not end of history)")
	}
	if res.Reached != 800 || res.RowsAdded != 2 || res.Chunks != 2 {
		t.Errorf("res = %+v, want Reached 800, 2 rows, 2 chunks", res)
	}

	// A run resumes from Reached.
	res2, err := b.BackfillBackward(ctx, res.Reached, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Reached != 700 || res2.RowsAdded != 1 {
		t.Errorf("resume res = %+v, want Reached 700, 1 row", res2)
	}
}

func TestBackfillBackwardStopsOnEmptyStreak(t *testing.T) {
	ctx := context.Background()
	w := newWriter(t)
	// History exists only at/above epoch 500; older windows come back empty.
	fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
		if start >= 500 {
			return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(20))}}, nil
		}
		return nil, nil
	}}
	b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(100), collect.WithEmptyStreakStop(2))

	res, err := b.BackfillBackward(ctx, 800, 0, 0) // unbounded
	if err != nil {
		t.Fatalf("BackfillBackward: %v", err)
	}
	if !res.Exhausted {
		t.Error("Exhausted = false, want true after the empty streak")
	}
	// Data at windows starting 700,600,500 (3 rows); then 400,300 empty -> stop.
	if res.RowsAdded != 3 {
		t.Errorf("RowsAdded = %d, want 3", res.RowsAdded)
	}
	if res.Reached != 300 {
		t.Errorf("Reached = %d, want 300 (second empty window start)", res.Reached)
	}
}

func TestBackfillBackwardStopsAtFloor(t *testing.T) {
	ctx := context.Background()
	w := newWriter(t)
	fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
		return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(20))}}, nil
	}}
	b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(100))

	res, err := b.BackfillBackward(ctx, 1000, 850, 0)
	if err != nil {
		t.Fatalf("BackfillBackward: %v", err)
	}
	// Second window is clamped to the floor rather than running past it.
	wantWindows := [][2]int64{{900, 999}, {850, 899}}
	if len(fetch.calls) != 2 || fetch.calls[1] != wantWindows[1] {
		t.Errorf("windows = %v, want the second clamped to %v", fetch.calls, wantWindows[1])
	}
	if !res.Exhausted || res.Reached != 850 {
		t.Errorf("res = %+v, want Exhausted at Reached 850", res)
	}
}

func TestBackfillBackwardFloorSurvivesOutage(t *testing.T) {
	ctx := context.Background()
	w := newWriter(t)
	// An outage from 400 to 799 spans four empty windows, longer than the
	// streak stop; with an explicit floor the walk must push through it and
	// only exhaust at the floor.
	fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
		if start >= 400 && start < 800 {
			return nil, nil
		}
		return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(20))}}, nil
	}}
	b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(100), collect.WithEmptyStreakStop(2))

	res, err := b.BackfillBackward(ctx, 1000, 100, 0)
	if err != nil {
		t.Fatalf("BackfillBackward: %v", err)
	}
	if !res.Exhausted || res.Reached != 100 {
		t.Errorf("res = %+v, want the walk to reach the floor at 100", res)
	}
	// Pre-outage data (100..300 window starts) must have been fetched.
	if res.RowsAdded != 5 {
		t.Errorf("RowsAdded = %d, want 5 (900,800 then 300,200,100)", res.RowsAdded)
	}
}

func TestBackfillRangeBoundedByMaxChunks(t *testing.T) {
	ctx := context.Background()
	w := newWriter(t)
	fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
		return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(20))}}, nil
	}}
	b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(100))

	res, err := b.BackfillRange(ctx, 1000, 1349, 2)
	if err != nil {
		t.Fatalf("BackfillRange: %v", err)
	}
	if res.Done {
		t.Error("Done = true, want false (budget spent with range remaining)")
	}
	if res.Resume != 1200 || res.RowsAdded != 2 {
		t.Errorf("res = %+v, want Resume 1200 with 2 rows", res)
	}

	// Resuming finishes the range.
	res2, err := b.BackfillRange(ctx, res.Resume, 1349, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Done || res2.RowsAdded != 2 {
		t.Errorf("resume res = %+v, want Done with the remaining 2 rows", res2)
	}
}

// TestCollectModeDispatch pins Collect's three-way mode choice: data present
// syncs incrementally, an empty archive with a start forward-fills, and an
// empty archive without one seeds by walking history backward.
func TestCollectModeDispatch(t *testing.T) {
	ctx := context.Background()

	t.Run("incremental", func(t *testing.T) {
		w := newWriter(t)
		if _, err := w.InsertObs(ctx, device, []model.DeviceObs{{Epoch: 5000, AirTempC: new(float64(20))}}); err != nil {
			t.Fatal(err)
		}
		fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
			return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(21))}}, nil
		}}
		b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(10_000))
		sum, err := b.Collect(ctx, 6000, 0)
		if err != nil {
			t.Fatalf("Collect: %v", err)
		}
		if sum.Mode != "incremental" {
			t.Errorf("Mode = %q, want incremental", sum.Mode)
		}
		if len(fetch.calls) == 0 || fetch.calls[0][0] != 5001 {
			t.Errorf("fetch windows = %v, want the first to start at 5001 (never re-walk stored history)", fetch.calls)
		}
	})

	t.Run("backfill from start", func(t *testing.T) {
		w := newWriter(t)
		fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
			return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(21))}}, nil
		}}
		b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(10_000))
		sum, err := b.Collect(ctx, 6000, 2000)
		if err != nil {
			t.Fatalf("Collect: %v", err)
		}
		if sum.Mode != "backfill" {
			t.Errorf("Mode = %q, want backfill", sum.Mode)
		}
		if len(fetch.calls) == 0 || fetch.calls[0][0] != 2000 {
			t.Errorf("fetch windows = %v, want the first to start at 2000", fetch.calls)
		}
	})

	t.Run("seed walks back", func(t *testing.T) {
		w := newWriter(t)
		// History exists only at/above 5500; the seed walk finds its end.
		fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
			if start >= 5500 {
				return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(21))}}, nil
			}
			return nil, nil
		}}
		b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(100), collect.WithEmptyStreakStop(2))
		sum, err := b.Collect(ctx, 6000, 0)
		if err != nil {
			t.Fatalf("Collect: %v", err)
		}
		if sum.Mode != "seed" {
			t.Errorf("Mode = %q, want seed", sum.Mode)
		}
		if sum.RowsAdded == 0 {
			t.Error("seed added no rows")
		}
	})
}

func TestSyncFromWatermark(t *testing.T) {
	ctx := context.Background()
	w := newWriter(t)

	// No data yet: Sync reports NoWatermark and fetches nothing.
	empty := &fakeFetcher{obsFor: func(_, _ int64) ([]model.DeviceObs, error) {
		t.Fatal("Sync must not fetch when the archive is empty")
		return nil, nil
	}}
	b := newBackfiller(t, empty, w)
	res, err := b.Sync(ctx, 10_000, 0)
	if err != nil {
		t.Fatalf("Sync (empty): %v", err)
	}
	if !res.NoWatermark || res.RowsAdded != 0 {
		t.Errorf("empty Sync = %+v, want NoWatermark with 0 rows", res)
	}

	// Seed a watermark at epoch 5000, then Sync forward to 6000.
	if _, err := w.InsertObs(ctx, device, []model.DeviceObs{{Epoch: 5000, AirTempC: new(float64(20))}}); err != nil {
		t.Fatal(err)
	}
	fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
		return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(21))}}, nil
	}}
	b = newBackfiller(t, fetch, w)
	res, err = b.Sync(ctx, 6000, 0)
	if err != nil {
		t.Fatalf("Sync (incremental): %v", err)
	}
	// Must fetch strictly after the watermark so stored data is never re-fetched.
	if len(fetch.calls) == 0 || fetch.calls[0][0] != 5001 {
		t.Errorf("first fetch window = %v, want it to start at 5001", fetch.calls)
	}
	if res.RowsAdded == 0 {
		t.Error("incremental Sync added no rows")
	}

	// Already current: watermark >= now means no fetch.
	fetch.calls = nil
	res, err = b.Sync(ctx, 5000, 0) // now equals the stored max
	if err != nil {
		t.Fatal(err)
	}
	if len(fetch.calls) != 0 || res.RowsAdded != 0 {
		t.Errorf("current Sync fetched %v / added %d, want none", fetch.calls, res.RowsAdded)
	}
}

func TestBackfillRangeResumesAfterError(t *testing.T) {
	ctx := context.Background()
	w := newWriter(t)

	boom := errors.New("boom")
	// Succeed on the first chunk, fail on the second (window start 1100).
	fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
		if start == 1100 {
			return nil, boom
		}
		return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(20))}}, nil
	}}
	b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(100))

	res, err := b.BackfillRange(ctx, 1000, 1349, 0)
	if err == nil {
		t.Fatal("expected an error from the failing chunk")
	}
	if res.Done {
		t.Error("Done = true, want false after an error")
	}
	if res.Resume != 1100 {
		t.Errorf("Resume = %d, want 1100 (failed chunk start)", res.Resume)
	}
	if res.RowsAdded != 1 {
		t.Errorf("RowsAdded = %d, want 1 (first chunk committed before the error)", res.RowsAdded)
	}
	if cov, err := w.Coverage(ctx, device); err != nil {
		t.Fatal(err)
	} else if cov.Count != 1 {
		t.Errorf("stored rows = %d, want 1 (first chunk durable despite failure)", cov.Count)
	}

	// Resume from where it stopped, now with a healthy fetcher: the rest lands
	// with no gap or duplicate.
	fetch.obsFor = func(start, _ int64) ([]model.DeviceObs, error) {
		return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(20))}}, nil
	}
	res2, err := b.BackfillRange(ctx, res.Resume, 1349, 0)
	if err != nil {
		t.Fatalf("resume BackfillRange: %v", err)
	}
	if !res2.Done || res2.RowsAdded != 3 {
		t.Errorf("resume res = %+v, want Done with 3 rows added", res2)
	}
	if cov, err := w.Coverage(ctx, device); err != nil {
		t.Fatal(err)
	} else if cov.Count != 4 {
		t.Errorf("final rows = %d, want 4", cov.Count)
	}
}

func TestCollectSeedInterruptedResumes(t *testing.T) {
	// The bug this guards against: a seed walk killed midway leaves a watermark,
	// so the next run switches to forward sync and the older history below the
	// interruption is silently never fetched. The persisted cursor closes that.
	ctx := context.Background()
	w := newWriter(t)

	// History spans [1000, 1999]; below that the API returns empty windows.
	history := func(start, _ int64) ([]model.DeviceObs, error) {
		if start >= 1000 {
			return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(21))}}, nil
		}
		return nil, nil
	}

	// First run: the fetch fails on the 4th backward chunk, after committing
	// windows starting 1900, 1800, 1700.
	failing := &fakeFetcher{obsFor: history}
	failing.obsFor = func(start, end int64) ([]model.DeviceObs, error) {
		if len(failing.calls) >= 4 {
			return nil, errors.New("network died")
		}
		return history(start, end)
	}
	b := newBackfiller(t, failing, w, collect.WithChunkSeconds(100), collect.WithEmptyStreakStop(2))
	if _, err := b.Collect(ctx, 2000, 0); err == nil {
		t.Fatal("want the interrupted seed to report its error")
	}
	cur, ok, err := w.Meta(ctx, collect.MetaBackfillCursor)
	if err != nil || !ok {
		t.Fatalf("cursor after interrupt: %q ok=%v err=%v", cur, ok, err)
	}
	if cur != "1700" {
		t.Fatalf("cursor = %q, want 1700 (oldest committed chunk start)", cur)
	}

	// Second run: sync forward, then resume the walk from the cursor without
	// re-fetching the chunks the first run already stored.
	resumed := &fakeFetcher{obsFor: history}
	b = newBackfiller(t, resumed, w, collect.WithChunkSeconds(100), collect.WithEmptyStreakStop(2))
	sum, err := b.Collect(ctx, 2000, 0)
	if err != nil {
		t.Fatalf("resumed Collect: %v", err)
	}
	if sum.Mode != "incremental+seed" {
		t.Errorf("Mode = %q, want incremental+seed", sum.Mode)
	}
	// First call is the forward sync from the watermark; the next is the walk
	// Resume below the cursor, not below the newest stored row.
	if len(resumed.calls) < 2 || resumed.calls[1] != [2]int64{1600, 1699} {
		t.Errorf("resume windows = %v, want the walk to continue at [1600,1699]", resumed.calls)
	}
	if v, ok, _ := w.Meta(ctx, collect.MetaBackfillComplete); !ok || v != "1" {
		t.Errorf("complete marker = %q ok=%v, want \"1\" after history ends", v, ok)
	}

	// A third run is purely incremental: the marker stops any further walking.
	third := &fakeFetcher{obsFor: history}
	b = newBackfiller(t, third, w, collect.WithChunkSeconds(100), collect.WithEmptyStreakStop(2))
	sum, err = b.Collect(ctx, 2100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Mode != "incremental" {
		t.Errorf("post-complete Mode = %q, want incremental", sum.Mode)
	}
}

func TestCollectBackfillStartNeverResumesSeed(t *testing.T) {
	// An archive forward-filled from an explicit start deliberately excludes
	// older history; later runs must not walk below it.
	ctx := context.Background()
	w := newWriter(t)
	fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
		return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(21))}}, nil
	}}
	b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(1000))
	if _, err := b.Collect(ctx, 6000, 2000); err != nil {
		t.Fatal(err)
	}
	sum, err := b.Collect(ctx, 7000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Mode != "incremental" {
		t.Errorf("Mode = %q, want incremental (no seed resume without a cursor)", sum.Mode)
	}
	for _, c := range fetch.calls {
		if c[0] < 2000 {
			t.Errorf("window %v reaches below the requested start", c)
		}
	}
}

func TestProgressReportsCumulativeChunks(t *testing.T) {
	ctx := context.Background()
	w := newWriter(t)
	fetch := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
		return []model.DeviceObs{{Epoch: start, AirTempC: new(float64(21))}}, nil
	}}
	var seen []collect.Progress
	b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(100),
		collect.WithProgress(func(p collect.Progress) error {
			seen = append(seen, p)
			return nil
		}))
	if _, err := b.BackfillRange(ctx, 1000, 1299, 0); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 {
		t.Fatalf("progress calls = %d, want one per chunk (3)", len(seen))
	}
	last := seen[len(seen)-1]
	if last.Fetched != 3 || last.RowsAdded != 3 || last.Chunks != 3 || last.Through != 1299 {
		t.Errorf("final progress = %+v, want cumulative 3/3/3 through 1299", last)
	}
}

type blockingFetcher struct {
	entered chan struct{}
	release chan struct{}
}

func (fetcher *blockingFetcher) DeviceObservations(ctx context.Context, _ int, start, _ int64) ([]model.DeviceObs, error) {
	select {
	case <-fetcher.entered:
	default:
		close(fetcher.entered)
	}
	select {
	case <-fetcher.release:
		return []model.DeviceObs{{Epoch: start}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestBackfillerRejectsConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	w := newWriter(t)
	fetcher := &blockingFetcher{entered: make(chan struct{}), release: make(chan struct{})}
	backfiller := newBackfiller(t, fetcher, w)
	firstDone := make(chan error, 1)
	go func() {
		_, err := backfiller.BackfillRange(ctx, 1000, 1000, 1)
		firstDone <- err
	}()
	<-fetcher.entered

	if _, err := backfiller.Sync(ctx, 1000, 1); !errors.Is(err, collect.ErrConcurrentUse) {
		t.Fatalf("concurrent Sync error = %v, want ErrConcurrentUse", err)
	}
	close(fetcher.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first operation: %v", err)
	}
}

func TestBackfillerCancellationStopsThrottle(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	w := newWriter(t)
	fetcher := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
		return []model.DeviceObs{{Epoch: start}}, nil
	}}
	backfiller := newBackfiller(t, fetcher, w,
		collect.WithChunkSeconds(1),
		collect.WithThrottle(time.Minute),
		collect.WithProgress(func(collect.Progress) error {
			cancel()
			return nil
		}),
	)
	_, err := backfiller.BackfillRange(ctx, 1000, 1001, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BackfillRange error = %v, want context cancellation", err)
	}
	if len(fetcher.calls) != 1 {
		t.Fatalf("fetch calls = %d, want one call before cancellation", len(fetcher.calls))
	}
}

func TestProgressFailureReportsCommittedResumePoint(t *testing.T) {
	progressErr := errors.New("progress sink failed")
	w := newWriter(t)
	fetcher := &fakeFetcher{obsFor: func(start, _ int64) ([]model.DeviceObs, error) {
		return []model.DeviceObs{{Epoch: start}}, nil
	}}
	backfiller := newBackfiller(t, fetcher, w,
		collect.WithChunkSeconds(1),
		collect.WithProgress(func(collect.Progress) error { return progressErr }),
	)
	result, err := backfiller.BackfillRange(t.Context(), 1000, 1001, 0)
	if !errors.Is(err, progressErr) {
		t.Fatalf("BackfillRange error = %v, want progress error", err)
	}
	if result.RowsAdded != 1 || result.Resume != 1001 || result.Done {
		t.Fatalf("result = %+v, want one committed row and resume 1001", result)
	}
	coverage, err := w.Coverage(t.Context(), device)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Count != 1 {
		t.Fatalf("stored rows = %d, want committed row retained", coverage.Count)
	}
}
