package mcpapp

// This file defines append-only archive tools. They require live access and a
// writable archive. Inserts use the device and observations returned by the API.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/collect"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// The walk-back cursor and complete marker live in pkg/tempest/collect so the
	// CLI and this server resume each other's backfills.
	metaBackfillCursor   = collect.MetaBackfillCursor
	metaBackfillComplete = collect.MetaBackfillComplete

	// gapThresholdSeconds is how far apart two consecutive observations must be to
	// count as a coverage gap. The archive holds 1-minute data, so an hour of
	// silence is genuine downtime worth surfacing.
	gapThresholdSeconds = 3600

	defaultBackfillMaxDays = 30  // history fetched per backfill_archive call by default
	maxBackfillMaxDays     = 365 // ceiling per call: at most 73 five-day requests
)

// politeThrottle is the pause between REST requests during a backfill or sync. No
// numeric rate limit is published for the Tempest API ("enough for personal use"),
// so we pace conservatively. Tunable via TEMPEST_THROTTLE_MS (0 disables; tests
// set 0 to run fast).
func politeThrottle() (time.Duration, error) {
	if s := os.Getenv("TEMPEST_THROTTLE_MS"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > 60_000 {
			return 0, fmt.Errorf("TEMPEST_THROTTLE_MS must be an integer in 0..60000")
		}
		return time.Duration(n) * time.Millisecond, nil
	}
	return 400 * time.Millisecond, nil
}

func newBackfiller(live *liveSource, w *store.Writer, deviceID int, options ...collect.Option) (*collect.Backfiller, error) {
	throttle, err := politeThrottle()
	if err != nil {
		return nil, err
	}
	return collect.New(live.client, w, deviceID, append([]collect.Option{collect.WithThrottle(throttle)}, options...)...)
}

// ---- output / input types ---------------------------------------------------

// ArchiveStatusOut is the archive_status output: coverage, freshness, and the
// largest gaps, so an agent can see what it has and target what it's missing.
type ArchiveStatusOut struct {
	Observations      int64       `json:"observations"`
	FirstObs          string      `json:"first_obs,omitempty"`
	LastObs           string      `json:"last_obs,omitempty"`
	SpanDays          float64     `json:"span_days"`
	LastObsAgeSeconds int64       `json:"last_obs_age_seconds"`
	Fresh             bool        `json:"fresh"` // last observation within the gap threshold of now
	BackfillComplete  bool        `json:"backfill_complete"`
	Gaps              []store.Gap `json:"gaps"`
	Note              string      `json:"note,omitempty"`
}

// SyncOut is the sync_archive output. Call again while has_more is true; a
// very stale archive catches up over several bounded calls.
type SyncOut struct {
	RowsAdded int    `json:"rows_added"`
	Fetched   int    `json:"fetched"`
	HasMore   bool   `json:"has_more"`
	LastObs   string `json:"last_obs,omitempty"`
	Note      string `json:"note,omitempty"`
}

// BackfillArgs is the backfill_archive input. With no arguments it walks history
// backward from where it left off, one bounded batch per call; start/end aim it
// at a specific older window.
type BackfillArgs struct {
	Start   string `json:"start,omitempty" jsonschema:"oldest date to fetch back to, YYYY-MM-DD local; the walk stops here"`
	End     string `json:"end,omitempty" jsonschema:"exclusive end date YYYY-MM-DD local (its midnight is not fetched); omit to resume the saved cursor"`
	MaxDays int    `json:"max_days,omitempty" jsonschema:"cap on 24-hour days of history fetched per call (default 30, max 365); batches may use fewer days to fit whole request windows"`
}

// BackfillOut is the backfill_archive output. Call again while has_more is true
// (no arguments needed; it resumes) until the whole history is stored.
type BackfillOut struct {
	RowsAdded    int    `json:"rows_added"`
	Fetched      int    `json:"fetched"`
	Chunks       int    `json:"chunks"`
	ReachedBack  string `json:"reached_back,omitempty"` // oldest date this call fetched to
	HasMore      bool   `json:"has_more"`
	NextBefore   int64  `json:"next_before,omitempty"` // Resume cursor in epoch seconds when has_more is true.
	Observations int64  `json:"observations"`          // total rows stored after this call
	Coverage     string `json:"coverage,omitempty"`    // first -> last stored
	Note         string `json:"note,omitempty"`
}

// ---- registration -----------------------------------------------------------

// registerArchiveStatusTool adds archive_status. It only reads, so it
// registers with any archive at all, keeping coverage visibility in read-only
// deployments where the write tools below are absent.
func registerArchiveStatusTool(srv *mcp.Server, st *store.Store) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "archive_status",
		Title:       "Archive status",
		Description: "Return row count, coverage, newest-row age, backfill state, and the ten largest gaps. This operation is read-only.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ NoArgs) (*mcp.CallToolResult, ArchiveStatusOut, error) {
		cov, err := st.Coverage(ctx)
		if err != nil {
			return nil, ArchiveStatusOut{}, err
		}
		out := ArchiveStatusOut{Observations: cov.Count}
		if cov.MinEpoch.Valid && cov.MaxEpoch.Valid {
			out.FirstObs = localTimeStr(cov.MinEpoch.Int64)
			out.LastObs = localTimeStr(cov.MaxEpoch.Int64)
			out.SpanDays = float64(cov.MaxEpoch.Int64-cov.MinEpoch.Int64) / 86400
			out.LastObsAgeSeconds = time.Now().Unix() - cov.MaxEpoch.Int64
			out.Fresh = out.LastObsAgeSeconds >= 0 && out.LastObsAgeSeconds < gapThresholdSeconds
		}
		gaps, err := st.Gaps(ctx, gapThresholdSeconds, 10)
		if err != nil {
			return nil, ArchiveStatusOut{}, err
		}
		out.Gaps = gaps
		complete, err := backfillComplete(ctx, st)
		if err != nil {
			return nil, ArchiveStatusOut{}, err
		}
		out.BackfillComplete = complete
		if cov.Count == 0 {
			out.Note = "archive is empty; run tempestkeep collect to seed history, or call backfill_archive if this server offers archive write tools"
		}
		return nil, out, nil
	})
}

// registerArchiveTools adds backfill_archive and sync_archive. Precondition
// (enforced by the caller): live and writer are non-nil.
func registerArchiveTools(srv *mcp.Server, live *liveSource, writer *store.Writer) {
	// MCP clients may issue concurrent calls. Serialize write workflows so a
	// slower request cannot overwrite a newer resume cursor or completion state.
	gate := make(chan struct{}, 1)
	acquire := func(ctx context.Context) error {
		select {
		case gate <- struct{}{}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "sync_archive",
		Title:       "Sync archive (append newest)",
		Description: "Append observations newer than the archive watermark. The operation is bounded and idempotent. Call it again while has_more is true.",
		Annotations: appendOnlyWrite(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ NoArgs) (*mcp.CallToolResult, SyncOut, error) {
		if err := acquire(ctx); err != nil {
			return nil, SyncOut{}, err
		}
		defer func() { <-gate }()
		deviceID, err := live.resolveDevice(ctx)
		if err != nil {
			return nil, SyncOut{}, err
		}
		// Bounded like backfill_archive: a months-stale archive syncs over
		// several calls, retaining progress through empty observation windows.
		r, err := syncBatch(ctx, live, writer, deviceID, time.Now().Unix())
		out := SyncOut{RowsAdded: r.RowsAdded, Fetched: r.Fetched, HasMore: !r.Done}
		if err != nil {
			return nil, out, err
		}
		switch {
		case r.NoWatermark:
			out.Note = "no data stored for this device yet; call backfill_archive to seed history, then sync_archive to keep it current"
		default:
			wm, ok, watermarkErr := writer.Watermark(ctx, deviceID)
			if watermarkErr != nil {
				return nil, out, watermarkErr
			}
			if ok {
				out.LastObs = localTimeStr(wm)
			}
			if out.HasMore {
				out.Note = "archive still behind; call sync_archive again to continue catching up"
			}
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "backfill_archive",
		Title:       "Backfill archive (download history)",
		Description: "Append older observations in one bounded batch. A call without dates resumes the saved cursor. max_days defaults to 30 and cannot exceed 365. Call again while has_more is true.",
		Annotations: appendOnlyWrite(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args BackfillArgs) (*mcp.CallToolResult, BackfillOut, error) {
		if args.MaxDays < 0 || args.MaxDays > maxBackfillMaxDays {
			return nil, BackfillOut{}, fmt.Errorf("max_days must be in 0..%d", maxBackfillMaxDays)
		}
		var floor int64
		if args.Start != "" {
			t, err := parseLocalDate(args.Start)
			if err != nil {
				return nil, BackfillOut{}, fmt.Errorf("invalid start: %w", err)
			}
			floor = t.Unix()
			if floor < 0 || floor > model.MaxEpochSeconds {
				return nil, BackfillOut{}, fmt.Errorf("start date is outside the supported observation range")
			}
			// Observations require a positive epoch. Reserve zero for the
			// open-ended heuristic even when the requested local date is epoch zero.
			floor = max(floor, 1)
			if args.End == "" && floor > time.Now().Unix() {
				return nil, BackfillOut{}, fmt.Errorf("start date must not be in the future when end is omitted")
			}
		}
		if args.End != "" {
			end, err := parseLocalDate(args.End)
			if err != nil {
				return nil, BackfillOut{}, fmt.Errorf("invalid end: %w", err)
			}
			if end.Unix() <= 0 || end.Unix() > model.MaxEpochSeconds {
				return nil, BackfillOut{}, fmt.Errorf("end date is outside the supported observation range")
			}
			if floor >= end.Unix() {
				return nil, BackfillOut{}, fmt.Errorf("start date must be before the exclusive end date")
			}
		}
		if err := acquire(ctx); err != nil {
			return nil, BackfillOut{}, err
		}
		defer func() { <-gate }()
		deviceID, err := live.resolveDevice(ctx)
		if err != nil {
			return nil, BackfillOut{}, err
		}
		if err := writer.BindDevice(ctx, deviceID); err != nil {
			return nil, BackfillOut{}, err
		}

		// Where to start walking back from, and whether to track the shared cursor.
		useCursor := args.End == ""
		before, err := backfillStartPoint(ctx, writer, deviceID, args.End, useCursor)
		if err != nil {
			return nil, BackfillOut{}, err
		}
		// A completed whole-history walk short-circuits, so re-calling is a no-op.
		if useCursor && floor == 0 {
			complete, metaErr := backfillComplete(ctx, writer)
			if metaErr != nil {
				return nil, BackfillOut{}, metaErr
			}
			if complete {
				out := BackfillOut{Note: "backfill already complete; call sync_archive to fetch newer data, or pass start/end to fill a specific older window"}
				if err := fillCoverage(ctx, writer, deviceID, &out); err != nil {
					return nil, out, err
				}
				return nil, out, nil
			}
		}

		r, err := backfillBatch(ctx, live, writer, deviceID, before, floor, args.MaxDays, useCursor)

		out := BackfillOut{RowsAdded: r.RowsAdded, Fetched: r.Fetched, Chunks: r.Chunks, HasMore: !r.Exhausted}
		if r.Chunks > 0 {
			out.ReachedBack = localDate(r.Reached)
		}
		if out.HasMore {
			out.NextBefore = r.Reached
		}
		if useCursor {
			// Only an open-ended walk can mark history collection complete;
			// the empty-history heuristic does not prove no older rows exist.
			// Stopping at a user-supplied floor says
			// nothing about what lies below it, so save the cursor instead
			// and let a later no-argument call keep walking from there.
			if r.Exhausted && floor == 0 {
				err = errors.Join(err, writer.SetMeta(ctx, metaBackfillComplete, "1"))
			}
		}
		err = errors.Join(err, fillCoverage(ctx, writer, deviceID, &out))
		if err != nil {
			return nil, out, err // partial progress is durable; the cursor lets a retry resume
		}
		if !out.HasMore {
			if floor > 0 {
				out.Note = "reached the requested start date; older history may still exist"
			} else {
				out.Note = "backfill stopped after 15 consecutive days without observations, or at the earliest supported timestamp; older data may exist beyond a long outage"
			}
		} else if !useCursor {
			out.Note = "this explicit end date does not advance the shared cursor; continue with an earlier end date to fetch an older window"
		}
		return nil, out, nil
	})
}

type metadataReader interface {
	Meta(context.Context, string) (string, bool, error)
}

func backfillComplete(ctx context.Context, metadata metadataReader) (bool, error) {
	value, exists, err := metadata.Meta(ctx, metaBackfillComplete)
	if err != nil || !exists {
		return false, err
	}
	switch value {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, fmt.Errorf("%w: archive backfill completion state is invalid", collect.ErrInvalidCheckpoint)
	}
}

// backfillStartPoint decides the exclusive upper epoch a walk-back begins from:
// an explicit end date, then the saved cursor, then below the oldest stored
// observation, else now (a fresh archive seeds from the present).
func backfillStartPoint(ctx context.Context, w *store.Writer, deviceID int, end string, useCursor bool) (int64, error) {
	if end != "" {
		t, err := parseLocalDate(end)
		if err != nil {
			return 0, fmt.Errorf("invalid end: %w", err)
		}
		return t.Unix(), nil
	}
	if useCursor {
		value, ok, err := w.Meta(ctx, metaBackfillCursor)
		if err != nil {
			return 0, err
		}
		if ok {
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 0 || n > model.MaxEpochSeconds {
				return 0, fmt.Errorf("%w: archive backfill cursor is invalid", collect.ErrInvalidCheckpoint)
			}
			return n, nil
		}
	}
	cov, err := w.Coverage(ctx, deviceID)
	if err != nil {
		return 0, err
	}
	if cov.MinEpoch.Valid {
		return cov.MinEpoch.Int64, nil
	}
	return time.Now().Unix(), nil
}

// backfillMaxChunks converts a max-days budget into a chunk count (each chunk is
// api.MaxDeviceWindow wide), clamped to sane bounds. It bounds the work one
// backfill_archive call does so the tool stays responsive.
func backfillMaxChunks(maxDays int) int {
	if maxDays <= 0 {
		maxDays = defaultBackfillMaxDays
	}
	if maxDays > maxBackfillMaxDays {
		maxDays = maxBackfillMaxDays
	}
	chunkDays := int64(api.MaxDeviceWindow / (24 * time.Hour))
	chunks := max(int64(maxDays)/chunkDays, 1)
	return int(chunks)
}

// fillCoverage annotates a BackfillOut with the archive's current total and span.
func fillCoverage(ctx context.Context, w *store.Writer, deviceID int, out *BackfillOut) error {
	cov, err := w.Coverage(ctx, deviceID)
	if err != nil {
		return err
	}
	out.Observations = cov.Count
	if cov.MinEpoch.Valid && cov.MaxEpoch.Valid {
		out.Coverage = localDate(cov.MinEpoch.Int64) + " -> " + localTimeStr(cov.MaxEpoch.Int64)
	}
	return nil
}
