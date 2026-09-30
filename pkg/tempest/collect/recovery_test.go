package collect_test

import (
	"errors"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/collect"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

func TestCollectPersistsSeedIntentBeforeFirstFetch(t *testing.T) {
	w := newWriter(t)
	fetchErr := errors.New("synthetic unavailable endpoint")
	fetch := &fakeFetcher{obsFor: func(_, _ int64) ([]model.DeviceObs, error) {
		cursor, ok, err := w.Meta(t.Context(), collect.MetaBackfillCursor)
		if err != nil || !ok || cursor != "1000" {
			t.Fatalf("cursor before first fetch = %q, %v, %v", cursor, ok, err)
		}
		complete, ok, err := w.Meta(t.Context(), collect.MetaBackfillComplete)
		if err != nil || !ok || complete != "0" {
			t.Fatalf("completion before first fetch = %q, %v, %v", complete, ok, err)
		}
		return nil, fetchErr
	}}
	b := newBackfiller(t, fetch, w)
	if _, err := b.Collect(t.Context(), 1000, 0); !errors.Is(err, fetchErr) {
		t.Fatalf("Collect error = %v, want fetch error", err)
	}
}

func TestCollectResumesSeedBeforeAnyObservation(t *testing.T) {
	w := newWriter(t)
	progressErr := errors.New("synthetic interrupted progress")
	fetch := &fakeFetcher{obsFor: func(_, _ int64) ([]model.DeviceObs, error) { return nil, nil }}
	b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(100),
		collect.WithProgress(func(collect.Progress) error { return progressErr }))
	if _, err := b.Collect(t.Context(), 1000, 0); !errors.Is(err, progressErr) {
		t.Fatalf("first Collect = %v, want progress error", err)
	}
	fetch.calls = nil
	b = newBackfiller(t, fetch, w, collect.WithChunkSeconds(100))
	if _, err := b.Collect(t.Context(), 1000, 0); err != nil {
		t.Fatal(err)
	}
	if len(fetch.calls) == 0 || fetch.calls[0] != [2]int64{800, 899} {
		t.Fatalf("resumed windows = %v, want first [800 899]", fetch.calls)
	}
}

func TestCollectResumesTerminalSeedCursor(t *testing.T) {
	w := newWriter(t)
	progressErr := errors.New("synthetic interrupted progress")
	fetch := &fakeFetcher{obsFor: func(_, _ int64) ([]model.DeviceObs, error) {
		return []model.DeviceObs{{Epoch: 1}}, nil
	}}
	b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(100),
		collect.WithProgress(func(collect.Progress) error { return progressErr }))
	if _, err := b.Collect(t.Context(), 100, 0); !errors.Is(err, progressErr) {
		t.Fatalf("first Collect = %v, want progress error", err)
	}
	cursor, ok, err := w.Meta(t.Context(), collect.MetaBackfillCursor)
	if err != nil || !ok || cursor != "0" {
		t.Fatalf("terminal cursor = %q, %v, %v", cursor, ok, err)
	}
	fetch.calls = nil
	fetch.obsFor = func(_, _ int64) ([]model.DeviceObs, error) { return nil, nil }
	b = newBackfiller(t, fetch, w, collect.WithChunkSeconds(100))
	if _, err := b.Collect(t.Context(), 100, 0); err != nil {
		t.Fatalf("resume terminal cursor: %v", err)
	}
	if len(fetch.calls) != 1 || fetch.calls[0] != [2]int64{2, 100} {
		t.Fatalf("resumed requests = %v, want forward sync only", fetch.calls)
	}
	complete, ok, err := w.Meta(t.Context(), collect.MetaBackfillComplete)
	if err != nil || !ok || complete != "1" {
		t.Fatalf("completion = %q, %v, %v", complete, ok, err)
	}
}

func TestBackfillRejectsOutOfWindowObservationsAtomically(t *testing.T) {
	for _, direction := range []string{"forward", "backward"} {
		t.Run(direction, func(t *testing.T) {
			w := newWriter(t)
			fetch := &fakeFetcher{obsFor: func(start, end int64) ([]model.DeviceObs, error) {
				return []model.DeviceObs{{Epoch: start}, {Epoch: end + 1}}, nil
			}}
			b := newBackfiller(t, fetch, w, collect.WithChunkSeconds(100))
			var err error
			if direction == "forward" {
				_, err = b.BackfillRange(t.Context(), 1000, 1099, 1)
			} else {
				_, err = b.BackfillBackward(t.Context(), 1100, 0, 1)
			}
			if !errors.Is(err, model.ErrInvalidObservation) {
				t.Fatalf("out-of-window response = %v, want ErrInvalidObservation", err)
			}
			coverage, err := w.Coverage(t.Context(), device)
			if err != nil || coverage.Count != 0 {
				t.Fatalf("rejected chunk changed observations: %+v, %v", coverage, err)
			}
		})
	}
}
