package mcpapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/collect"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

const (
	metaBackfillEmptySpan = "mcp_backfill_empty_span"
	emptyHistorySeconds   = int64(collect.DefaultEmptyStreakStop * api.MaxDeviceWindow / time.Second)
)

var errEmptyHistory = errors.New("empty history limit reached")

// backfillEmptySpan belongs to exactly one shared cursor. If another collector
// advances the cursor, its stale empty-span state is discarded. This avoids
// declaring history exhausted using requests that were never made.
type backfillEmptySpan struct {
	Before       int64 `json:"before"`
	EmptySeconds int64 `json:"empty_seconds"`
}

// backfillBatch bounds the requested history and retains the empty-history
// heuristic across small batches and server restarts. Cursor metadata advances
// after every committed chunk; a canceled call can safely replay its last chunk.
func backfillBatch(ctx context.Context, live *liveSource, writer *store.Writer, deviceID int, before, floor int64, maxDays int, useCursor bool) (collect.BackwardResult, error) {
	result := collect.BackwardResult{Reached: before}
	if before <= floor {
		result.Exhausted = true
		return result, nil
	}
	state := backfillEmptySpan{Before: before}
	if useCursor && floor == 0 {
		value, ok, err := writer.Meta(ctx, metaBackfillEmptySpan)
		if err != nil {
			return result, err
		}
		if ok {
			var saved backfillEmptySpan
			if err := json.Unmarshal([]byte(value), &saved); err != nil || saved.Before < 0 || saved.EmptySeconds < 0 || saved.EmptySeconds > emptyHistorySeconds {
				return result, fmt.Errorf("%w: archive empty-history state is invalid", collect.ErrInvalidCheckpoint)
			}
			if saved.Before == before {
				state = saved
			}
		}
	}
	if state.EmptySeconds == emptyHistorySeconds {
		result.Exhausted = true
		return result, nil
	}
	if maxDays == 0 {
		maxDays = defaultBackfillMaxDays
	}
	chunkSeconds := min(int64(maxDays)*86400, int64(api.MaxDeviceWindow/time.Second))
	previousFetched := 0
	progress := func(p collect.Progress) error {
		if floor == 0 && p.Fetched == previousFetched {
			state.EmptySeconds = min(state.EmptySeconds+state.Before-p.Through, emptyHistorySeconds)
		} else {
			state.EmptySeconds = 0
		}
		previousFetched = p.Fetched
		state.Before = p.Through
		if useCursor {
			value, err := json.Marshal(state)
			if err != nil {
				return err
			}
			// Write the heuristic first. If saving the cursor fails, the old
			// cursor will not match this state and a retry safely resets it.
			if err := writer.SetMeta(ctx, metaBackfillEmptySpan, string(value)); err != nil {
				return err
			}
			if err := writer.SetMeta(ctx, metaBackfillCursor, strconv.FormatInt(p.Through, 10)); err != nil {
				return err
			}
		}
		if state.EmptySeconds == emptyHistorySeconds {
			return errEmptyHistory
		}
		return nil
	}
	// The collector counts empty windows within a call. Our shorter windows
	// must still cover at least the normal 15-day empty-history threshold.
	emptyWindows := int((emptyHistorySeconds + chunkSeconds - 1) / chunkSeconds)
	bf, err := newBackfiller(live, writer, deviceID,
		collect.WithChunkSeconds(chunkSeconds), collect.WithEmptyStreakStop(emptyWindows), collect.WithProgress(progress))
	if err != nil {
		return result, err
	}
	if useCursor {
		// A new bounded walk can deliberately look past an earlier heuristic
		// stop. Its old completion marker must not suppress a later resume.
		if err := writer.SetMeta(ctx, metaBackfillComplete, "0"); err != nil {
			return result, err
		}
		// Preserve the initial upper bound before any rows can commit, including
		// when the first chunk's checkpoint fails after its observation insert.
		if err := writer.SetMeta(ctx, metaBackfillCursor, strconv.FormatInt(before, 10)); err != nil {
			return result, err
		}
	}
	result, err = bf.BackfillBackward(ctx, before, floor, backfillMaxChunks(maxDays))
	if errors.Is(err, errEmptyHistory) {
		result.Exhausted = true
		err = nil
	}
	return result, err
}
