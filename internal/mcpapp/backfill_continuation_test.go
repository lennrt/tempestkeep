package mcpapp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBackfillExactContinuationAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	previous := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = previous })
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	start := time.Date(2025, time.November, 2, 0, 0, 0, 0, loc).Unix()
	end := time.Date(2025, time.November, 3, 0, 0, 0, 0, loc).Unix()
	boundary := end - 86400 // 01:00 on the 25-hour local day.
	history := &requestedHistory{epochs: []int64{start, boundary - 1, boundary, end - 1}}
	_ = batchTestSource(t, history)
	cs := connectWritableServer(t, ctx, "synthetic-token", filepath.Join(t.TempDir(), "archive.sqlite"))
	var first BackfillOut
	callTool(t, ctx, cs, "backfill_archive", map[string]any{
		"start": "2025-11-02", "end": "2025-11-03", "max_days": 1,
	}, &first)
	if !first.HasMore || first.NextBefore != boundary || first.RowsAdded != 2 {
		t.Fatalf("first batch = %+v, want two observations and cursor %d", first, boundary)
	}
	var second BackfillOut
	callTool(t, ctx, cs, "backfill_archive", map[string]any{
		"start": "2025-11-02", "before_epoch": first.NextBefore, "max_days": 1,
	}, &second)
	if !strings.Contains(first.Note, "before_epoch") || !strings.Contains(first.Note, "start") || !strings.Contains(first.Note, "max_days") {
		t.Fatalf("continuation instructions omit required arguments: %q", first.Note)
	}
	if second.HasMore || second.NextBefore != 0 || second.RowsAdded != 2 || second.Observations != 4 {
		t.Fatalf("final batch = %+v, want all four observations and exact floor completion", second)
	}
	windows := history.requests()
	if len(windows) != 2 || windows[0] != [2]int64{boundary, end - 1} || windows[1] != [2]int64{start, boundary - 1} {
		t.Fatalf("non-adjacent or incorrectly bounded requests: %v", windows)
	}
	var replay BackfillOut
	callTool(t, ctx, cs, "backfill_archive", map[string]any{
		"start": "2025-11-02", "before_epoch": first.NextBefore, "max_days": 1,
	}, &replay)
	if replay.RowsAdded != 0 || replay.Observations != 4 || replay.HasMore {
		t.Fatalf("replayed continuation was not idempotent: %+v", replay)
	}
}

func TestBackfillExactContinuationPreservesSharedState(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const before = int64(1700000123) // Deliberately not a date or minute boundary.
	history := &requestedHistory{epochs: []int64{before - 1, before}}
	_ = batchTestSource(t, history)
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	writer, err := store.OpenWriter(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	state := map[string]string{
		metaBackfillCursor: "1690000000", metaBackfillComplete: "1",
		metaBackfillEmptySpan: `{"before":1690000000,"empty_seconds":43200}`,
		metaSyncCursor:        `{"watermark":1690000000,"next":1690000100}`,
	}
	for key, value := range state {
		if err := writer.SetMeta(ctx, key, value); err != nil {
			t.Fatal(err)
		}
	}
	cs := connectWritableServer(t, ctx, "synthetic-token", path)
	var result BackfillOut
	callTool(t, ctx, cs, "backfill_archive", map[string]any{"before_epoch": before, "max_days": 1}, &result)
	if !result.HasMore || result.NextBefore != before-86400 || result.RowsAdded != 1 {
		t.Fatalf("exact exclusive cursor result = %+v", result)
	}
	windows := history.requests()
	if len(windows) != 1 || windows[0] != [2]int64{before - 86400, before - 1} {
		t.Fatalf("requested windows = %v", windows)
	}
	// Date-based explicit calls must preserve the same state as epoch calls.
	callTool(t, ctx, cs, "backfill_archive", map[string]any{"end": "2023-01-03", "max_days": 1}, &result)
	for key, want := range state {
		got, ok, err := writer.Meta(ctx, key)
		if err != nil || !ok || got != want {
			t.Errorf("explicit request changed %s: got %q, present=%v, err=%v, want %q", key, got, ok, err, want)
		}
	}
}

func TestBackfillExactContinuationRejectsInvalidBeforeIO(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	history := &requestedHistory{}
	_ = batchTestSource(t, history)
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	cs := connectWritableServer(t, ctx, "synthetic-token", path)
	start, err := parseLocalDate("2024-01-01")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []map[string]any{
		{"before_epoch": 0}, {"before_epoch": -1}, {"before_epoch": model.MaxEpochSeconds + 1},
		{"before_epoch": 1.5}, {"before_epoch": "1700000000"},
		{"before_epoch": start.Unix(), "end": "2024-01-02"},
		{"before_epoch": start.Unix(), "start": "2024-01-01"},
		{"before_epoch": start.Unix() - 1, "start": "2024-01-01"},
		{"before_epoch": start.Unix(), "start": "invalid"},
	} {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "backfill_archive", Arguments: args})
		if err != nil || !result.IsError {
			t.Fatalf("args=%v, result=%+v, err=%v", args, result, err)
		}
	}
	if windows := history.requests(); len(windows) != 0 {
		t.Fatalf("invalid arguments fetched observations: %v", windows)
	}
	history.mu.Lock()
	stationRequests := history.stationRequests
	history.mu.Unlock()
	if stationRequests != 0 {
		t.Fatalf("invalid arguments made %d discovery requests", stationRequests)
	}
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, st)
	if _, exists, err := st.Meta(ctx, "archive_device_id"); err != nil || exists {
		t.Fatalf("invalid arguments bound the archive: exists=%v, err=%v", exists, err)
	}
}

func TestBackfillExactContinuationReplaysAfterPartialFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	start, err := parseLocalDate("2023-01-01")
	if err != nil {
		t.Fatal(err)
	}
	before := start.Unix() + 10*86400
	history := &requestedHistory{
		epochs: []int64{start.Unix(), before - 1},
		failAt: start.Unix(), // The second five-day chunk fails.
	}
	_ = batchTestSource(t, history)
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	cs := connectWritableServer(t, ctx, "synthetic-token", path)
	args := map[string]any{"start": "2023-01-01", "before_epoch": before, "max_days": 10}
	failed, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "backfill_archive", Arguments: args})
	if err != nil || !failed.IsError {
		t.Fatalf("partial request should fail: result=%+v, err=%v", failed, err)
	}
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, st)
	cov, err := st.Coverage(ctx)
	if err != nil || cov.Count != 1 {
		t.Fatalf("first chunk was not durable: coverage=%+v, err=%v", cov, err)
	}
	history.mu.Lock()
	history.failAt = 0
	history.mu.Unlock()
	var retried BackfillOut
	callTool(t, ctx, cs, "backfill_archive", args, &retried)
	if retried.HasMore || retried.RowsAdded != 1 || retried.Observations != 2 {
		t.Fatalf("retry did not complete the range safely: %+v", retried)
	}
	windows := history.requests()
	if len(windows) != 4 || windows[0] != windows[2] || windows[1] != windows[3] {
		t.Fatalf("retry did not replay the failed explicit window: %v", windows)
	}
	for _, key := range []string{metaBackfillCursor, metaBackfillComplete, metaBackfillEmptySpan, metaSyncCursor} {
		if _, exists, err := st.Meta(ctx, key); err != nil || exists {
			t.Errorf("explicit request created %s: exists=%v, err=%v", key, exists, err)
		}
	}
}
