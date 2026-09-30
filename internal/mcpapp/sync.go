package mcpapp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lennrt/tempestkeep/pkg/tempest/collect"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

const metaSyncCursor = "mcp_sync_cursor"

// syncCursor records successfully scanned empty windows too. A watermark alone
// cannot advance through an outage longer than one bounded sync call. Another
// writer moving the watermark invalidates this hint, so continuation restarts
// conservatively after the newest stored observation.
type syncCursor struct {
	Watermark int64 `json:"watermark"`
	Next      int64 `json:"next"`
}

func syncBatch(ctx context.Context, live *liveSource, writer *store.Writer, deviceID int, now int64) (collect.Result, error) {
	if now <= 0 || now > model.MaxEpochSeconds {
		return collect.Result{}, fmt.Errorf("%w: sync timestamp is outside the supported observation range", collect.ErrInvalidConfig)
	}
	progress := func(p collect.Progress) error {
		watermark, ok, err := writer.Watermark(ctx, deviceID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: archive watermark disappeared during sync", collect.ErrInvalidCheckpoint)
		}
		state := syncCursor{Watermark: watermark, Next: max(watermark+1, p.Through+1)}
		if p.Through == now {
			// A completed scan must recheck its empty tail next time: the live
			// API can publish recent observations after our current request.
			state.Next = watermark + 1
		}
		value, err := json.Marshal(state)
		if err != nil {
			return err
		}
		return writer.SetMeta(ctx, metaSyncCursor, string(value))
	}
	bf, err := newBackfiller(live, writer, deviceID, collect.WithProgress(progress))
	if err != nil {
		return collect.Result{}, err
	}
	if err := writer.BindDevice(ctx, deviceID); err != nil {
		return collect.Result{}, err
	}
	watermark, haveWatermark, err := writer.Watermark(ctx, deviceID)
	if err != nil {
		return collect.Result{}, err
	}
	if !haveWatermark {
		return collect.Result{Done: true, NoWatermark: true, Resume: now}, nil
	}
	next := watermark + 1
	value, ok, err := writer.Meta(ctx, metaSyncCursor)
	if err != nil {
		return collect.Result{}, err
	}
	if ok {
		var saved syncCursor
		if err := json.Unmarshal([]byte(value), &saved); err != nil || saved.Watermark <= 0 || saved.Watermark > model.MaxEpochSeconds || saved.Next <= saved.Watermark || saved.Next > model.MaxEpochSeconds+1 {
			return collect.Result{}, fmt.Errorf("%w: archive forward sync state is invalid", collect.ErrInvalidCheckpoint)
		}
		if saved.Watermark == watermark {
			next = saved.Next
		}
	}
	if next > now {
		return collect.Result{Done: true, Resume: next}, nil
	}
	return bf.BackfillRange(ctx, next, now, backfillMaxChunks(0))
}
