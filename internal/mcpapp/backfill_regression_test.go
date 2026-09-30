package mcpapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/collect"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type requestedHistory struct {
	mu              sync.Mutex
	windows         [][2]int64
	epochs          []int64
	failAt          int64
	failFrom        int64
	stationRequests int
}

func (h *requestedHistory) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/stations" {
			h.mu.Lock()
			h.stationRequests++
			h.mu.Unlock()
			_, _ = w.Write([]byte(e2eStationsJSON))
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/observations/device/") {
			http.NotFound(w, r)
			return
		}
		start, _ := strconv.ParseInt(r.URL.Query().Get("time_start"), 10, 64)
		end, _ := strconv.ParseInt(r.URL.Query().Get("time_end"), 10, 64)
		h.mu.Lock()
		defer h.mu.Unlock()
		h.windows = append(h.windows, [2]int64{start, end})
		if h.failAt > 0 && start <= h.failAt || h.failFrom > 0 && start >= h.failFrom {
			_, _ = w.Write([]byte(`{"obs":`))
			return
		}
		var epochs []int64
		for _, epoch := range h.epochs {
			if epoch >= start && epoch <= end {
				epochs = append(epochs, epoch)
			}
		}
		writeObs(w, epochs)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (h *requestedHistory) requests() [][2]int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([][2]int64(nil), h.windows...)
}

func batchTestSource(t *testing.T, h *requestedHistory) *liveSource {
	t.Helper()
	srv := h.server(t)
	t.Setenv("TEMPEST_API_BASE", srv.URL)
	t.Setenv("TEMPEST_CACHE_TTL", "0")
	t.Setenv("TEMPEST_THROTTLE_MS", "0")
	client, err := newAPIClient("synthetic-token")
	if err != nil {
		t.Fatal(err)
	}
	return &liveSource{client: client}
}

func TestBackfillSmallBudgetsTerminateAcrossRestarts(t *testing.T) {
	const initial = int64(1700000000)
	for _, budget := range []int{1, 6} {
		t.Run(strconv.Itoa(budget), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			history := &requestedHistory{epochs: []int64{initial - 86400, initial - 4*86400, initial - 10*86400}}
			live := batchTestSource(t, history)
			path := filepath.Join(t.TempDir(), "archive.sqlite")
			before := initial
			finished := false
			for call := range 30 {
				writer, err := store.OpenWriter(ctx, path)
				if err != nil {
					t.Fatal(err)
				}
				if call > 0 {
					before, err = backfillStartPoint(ctx, writer, 456, "", true)
					if err != nil {
						t.Fatal(err)
					}
				}
				result, err := backfillBatch(ctx, live, writer, 456, before, 0, budget, true)
				if err != nil {
					t.Fatal(err)
				}
				if before-result.Reached > int64(budget)*86400 {
					t.Fatalf("budget %d fetched %d seconds", budget, before-result.Reached)
				}
				if result.Exhausted {
					if initial-result.Reached < 25*86400 {
						t.Fatal("history ended before 15 empty days after the final observation window")
					}
					coverage, err := writer.Coverage(ctx, 456)
					if err != nil || coverage.Count != 3 {
						t.Fatalf("coverage=%+v, err=%v", coverage, err)
					}
					finished = true
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				if finished {
					break
				}
			}
			if !finished {
				t.Fatal("small-budget backfill never exhausted history")
			}
			windows := history.requests()
			for i, window := range windows {
				if window[1]-window[0]+1 > int64(budget)*86400 {
					t.Fatalf("request %v exceeds budget %d", window, budget)
				}
				if i > 0 && window[1]+1 != windows[i-1][0] {
					t.Fatalf("resume skipped or repeated an interval: %v then %v", windows[i-1], window)
				}
			}
		})
	}
}

func TestBackfillExplicitFloorCrossesLongEmptyGap(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const initial = int64(1700000000)
	live := batchTestSource(t, &requestedHistory{})
	writer, err := store.OpenWriter(ctx, filepath.Join(t.TempDir(), "archive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	floor := initial - 21*86400
	before := initial
	for call := range 21 {
		result, err := backfillBatch(ctx, live, writer, 456, before, floor, 1, true)
		if err != nil {
			t.Fatal(err)
		}
		if result.Exhausted != (call == 20) {
			t.Fatalf("call %d exhausted=%v, want completion only at the explicit floor", call, result.Exhausted)
		}
		before = result.Reached
	}
	if before != floor {
		t.Fatalf("stopped at %d, want %d", before, floor)
	}
}

func TestBackfillFailurePreservesCommittedCursor(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const initial = int64(1700000000)
	history := &requestedHistory{epochs: []int64{initial - 1}, failAt: initial - 10*86400}
	live := batchTestSource(t, history)
	writer, err := store.OpenWriter(ctx, filepath.Join(t.TempDir(), "archive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	result, err := backfillBatch(ctx, live, writer, 456, initial, 0, 10, true)
	if err == nil || result.RowsAdded != 1 || result.Chunks != 1 {
		t.Fatalf("partial result=%+v, err=%v", result, err)
	}
	cursor, err := backfillStartPoint(ctx, writer, 456, "", true)
	if err != nil || cursor != initial-5*86400 {
		t.Fatalf("cursor=%d, err=%v", cursor, err)
	}
	history.mu.Lock()
	history.failAt = 0
	history.mu.Unlock()
	resumed, err := backfillBatch(ctx, live, writer, 456, cursor, 0, 1, true)
	if err != nil || resumed.Reached != cursor-86400 || resumed.RowsAdded != 0 {
		t.Fatalf("resumed=%+v, err=%v", resumed, err)
	}
}

func TestBackfillRejectsCorruptCursor(t *testing.T) {
	writer, err := store.OpenWriter(t.Context(), filepath.Join(t.TempDir(), "archive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	for _, value := range []string{"not-an-epoch", "-1", "253402300800"} {
		if err := writer.SetMeta(t.Context(), metaBackfillCursor, value); err != nil {
			t.Fatal(err)
		}
		if _, err := backfillStartPoint(t.Context(), writer, 456, "", true); !errors.Is(err, collect.ErrInvalidCheckpoint) {
			t.Fatalf("cursor %q: got %v, want invalid checkpoint", value, err)
		}
	}
}

func TestBackfillInvalidDatesMakeNoAPIRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	history := &requestedHistory{}
	_ = batchTestSource(t, history)
	cs := connectWritableServer(t, ctx, "synthetic-token", filepath.Join(t.TempDir(), "archive.sqlite"))
	for _, args := range []map[string]any{
		{"start": "not-a-date"}, {"end": "not-a-date"},
		{"start": "2024-01-02", "end": "2024-01-01"},
		{"start": "2024-01-01", "end": "2024-01-01"},
		{"start": "1960-01-01"}, {"end": "1960-01-01"},
		{"start": time.Now().AddDate(0, 0, 2).Format(time.DateOnly)},
	} {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "backfill_archive", Arguments: args})
		if err != nil || !result.IsError {
			t.Fatalf("args=%v, result=%+v, err=%v", args, result, err)
		}
	}
	if requests := history.requests(); len(requests) != 0 {
		t.Fatalf("invalid dates fetched history: %v", requests)
	}
	history.mu.Lock()
	defer history.mu.Unlock()
	if history.stationRequests != 0 {
		t.Fatalf("invalid dates made %d station requests", history.stationRequests)
	}
}

func TestBackfillEmptySmallBatchesMarkComplete(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	history := &requestedHistory{}
	_ = batchTestSource(t, history)
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	cs := connectWritableServer(t, ctx, "synthetic-token", path)
	for call := range 15 {
		var result BackfillOut
		callTool(t, ctx, cs, "backfill_archive", map[string]any{"max_days": 1}, &result)
		if result.HasMore != (call < 14) {
			t.Fatalf("call %d has_more=%v", call, result.HasMore)
		}
	}
	var status ArchiveStatusOut
	callTool(t, ctx, cs, "archive_status", nil, &status)
	if !status.BackfillComplete || status.Observations != 0 {
		t.Fatalf("status=%+v", status)
	}
	var repeated BackfillOut
	callTool(t, ctx, cs, "backfill_archive", map[string]any{"max_days": 1}, &repeated)
	if repeated.HasMore || repeated.Chunks != 0 || len(history.requests()) != 15 {
		t.Fatalf("completed backfill repeated work: %+v", repeated)
	}
}

func TestBackfillDiscardsEmptySpanFromAnotherCursor(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const before = int64(1700000000)
	live := batchTestSource(t, &requestedHistory{})
	writer, err := store.OpenWriter(ctx, filepath.Join(t.TempDir(), "archive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	state, err := json.Marshal(backfillEmptySpan{Before: before + 1, EmptySeconds: emptyHistorySeconds})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.SetMeta(ctx, metaBackfillEmptySpan, string(state)); err != nil {
		t.Fatal(err)
	}
	result, err := backfillBatch(ctx, live, writer, 456, before, 0, 1, true)
	if err != nil || result.Exhausted || result.Chunks != 1 {
		t.Fatalf("stale state caused early exhaustion: %+v, %v", result, err)
	}
}

func TestBackfillExplicitEndDoesNotMoveSharedCursor(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	history := &requestedHistory{}
	_ = batchTestSource(t, history)
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	writer, err := store.OpenWriter(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	const sharedCursor = "1700000000"
	if err := writer.SetMeta(ctx, metaBackfillCursor, sharedCursor); err != nil {
		t.Fatal(err)
	}
	cs := connectWritableServer(t, ctx, "synthetic-token", path)
	var result BackfillOut
	callTool(t, ctx, cs, "backfill_archive", map[string]any{"start": "2023-01-01", "end": "2023-01-03", "max_days": 1}, &result)
	value, _, err := writer.Meta(ctx, metaBackfillCursor)
	if err != nil || value != sharedCursor || !result.HasMore || !strings.Contains(result.Note, "explicit end") {
		t.Fatalf("cursor=%q, result=%+v, err=%v", value, result, err)
	}
	start, _ := parseLocalDate("2023-01-02")
	end, _ := parseLocalDate("2023-01-03")
	windows := history.requests()
	if len(windows) != 1 || windows[0] != [2]int64{start.Unix(), end.Unix() - 1} {
		t.Fatalf("requested windows = %v", windows)
	}
}

func TestBackfillDeviceMismatchPreservesMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	history := &requestedHistory{}
	_ = batchTestSource(t, history)
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	writer, err := store.OpenWriter(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	if err := writer.BindDevice(ctx, 999); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{metaBackfillCursor: "1700000000", metaBackfillComplete: "1"} {
		if err := writer.SetMeta(ctx, key, value); err != nil {
			t.Fatal(err)
		}
	}
	cs := connectWritableServer(t, ctx, "synthetic-token", path)
	for _, args := range []map[string]any{{}, {"start": "2023-01-01"}} {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "backfill_archive", Arguments: args})
		if err != nil || !result.IsError {
			t.Fatalf("mismatched device: result=%+v err=%v", result, err)
		}
	}
	for key, want := range map[string]string{metaBackfillCursor: "1700000000", metaBackfillComplete: "1"} {
		got, _, err := writer.Meta(ctx, key)
		if err != nil || got != want {
			t.Fatalf("%s changed: got %q want %q err=%v", key, got, want, err)
		}
	}
	if len(history.requests()) != 0 {
		t.Fatal("mismatched device fetched observation history")
	}
}

func TestBackfillExplicitStartClearsOldCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	history := &requestedHistory{}
	_ = batchTestSource(t, history)
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	writer, err := store.OpenWriter(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	before, err := parseLocalDate("2023-01-03")
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{metaBackfillCursor: strconv.FormatInt(before.Unix(), 10), metaBackfillComplete: "1"} {
		if err := writer.SetMeta(ctx, key, value); err != nil {
			t.Fatal(err)
		}
	}
	cs := connectWritableServer(t, ctx, "synthetic-token", path)
	var bounded BackfillOut
	callTool(t, ctx, cs, "backfill_archive", map[string]any{"start": "2023-01-02", "max_days": 1}, &bounded)
	if bounded.HasMore {
		t.Fatal("bounded walk did not reach the requested floor")
	}
	var status ArchiveStatusOut
	callTool(t, ctx, cs, "archive_status", nil, &status)
	if status.BackfillComplete {
		t.Fatal("explicit floor retained an earlier whole-history completion marker")
	}
	var resumed BackfillOut
	callTool(t, ctx, cs, "backfill_archive", map[string]any{"max_days": 1}, &resumed)
	if resumed.Chunks != 1 || !resumed.HasMore || len(history.requests()) != 2 {
		t.Fatalf("open-ended walk did not resume below floor: %+v", resumed)
	}
}

func TestBackfillQueuedCallCancelsWithoutFetching(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/stations" {
			_, _ = w.Write([]byte(e2eStationsJSON))
			return
		}
		if requests.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			writeObs(w, nil)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(unblock)
	t.Setenv("TEMPEST_API_BASE", srv.URL)
	t.Setenv("TEMPEST_CACHE_TTL", "0")
	t.Setenv("TEMPEST_THROTTLE_MS", "0")
	cs := connectWritableServer(t, ctx, "synthetic-token", filepath.Join(t.TempDir(), "archive.sqlite"))
	first := make(chan error, 1)
	go func() {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "backfill_archive", Arguments: map[string]any{"max_days": 1}})
		if err == nil && result.IsError {
			err = errors.New("first backfill returned a tool error")
		}
		first <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("first backfill never reached the API")
	}
	queuedCtx, stopQueued := context.WithTimeout(ctx, 100*time.Millisecond)
	defer stopQueued()
	_, err := cs.CallTool(queuedCtx, &mcp.CallToolParams{Name: "backfill_archive", Arguments: map[string]any{"max_days": 1}})
	if err == nil {
		t.Fatal("queued call did not report cancellation")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("queued call made an overlapping API request: requests=%d", got)
	}
	unblock()
	select {
	case err := <-first:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("first backfill did not finish after release")
	}
	var next BackfillOut
	callTool(t, ctx, cs, "backfill_archive", map[string]any{"max_days": 1}, &next)
	if requests.Load() != 2 || next.Chunks != 1 {
		t.Fatalf("write gate did not recover after cancellation: requests=%d result=%+v", requests.Load(), next)
	}
}

func TestArchiveStatusDoesNotLabelFutureObservationFresh(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	writer, err := store.OpenWriter(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	if _, err := writer.InsertObs(ctx, 456, []model.DeviceObs{{Epoch: time.Now().Add(time.Hour).Unix()}}); err != nil {
		t.Fatal(err)
	}
	cs := connectArchiveServer(t, ctx, path)
	var status ArchiveStatusOut
	callTool(t, ctx, cs, "archive_status", nil, &status)
	if status.Fresh || status.LastObsAgeSeconds >= 0 {
		t.Fatalf("future observation reported fresh: %+v", status)
	}
}

func TestBackfillRejectsCorruptCompletionState(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	history := &requestedHistory{}
	_ = batchTestSource(t, history)
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	writer, err := store.OpenWriter(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	if err := writer.SetMeta(ctx, metaBackfillComplete, "invalid"); err != nil {
		t.Fatal(err)
	}
	cs := connectWritableServer(t, ctx, "synthetic-token", path)
	for _, tool := range []string{"archive_status", "backfill_archive"} {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: map[string]any{}})
		if err != nil || !result.IsError {
			t.Fatalf("%s accepted corrupt completion state: %+v, %v", tool, result, err)
		}
	}
	if len(history.requests()) != 0 {
		t.Fatal("corrupt checkpoint triggered observation requests")
	}
}

func TestSyncCrossesEmptyBudgetAcrossCalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	now := time.Now().Unix()
	history := &requestedHistory{epochs: []int64{now - 86400}}
	_ = batchTestSource(t, history)
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	writer, err := store.OpenWriter(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	if _, err := writer.InsertObs(ctx, 456, []model.DeviceObs{{Epoch: now - 40*86400}}); err != nil {
		t.Fatal(err)
	}
	cs := connectWritableServer(t, ctx, "synthetic-token", path)
	var first, second SyncOut
	callTool(t, ctx, cs, "sync_archive", nil, &first)
	callTool(t, ctx, cs, "sync_archive", nil, &second)
	if !first.HasMore || first.RowsAdded != 0 || second.HasMore || second.RowsAdded != 1 {
		t.Fatalf("sync could not cross a 30-day data gap: first=%+v second=%+v windows=%v", first, second, history.requests())
	}
}

func TestSyncCursorSurvivesRestartAndAPIError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const initial = int64(1700000000)
	const now = initial + 70*86400
	history := &requestedHistory{epochs: []int64{now - 86400}, failFrom: initial + 10*86400}
	live := batchTestSource(t, history)
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	writer, err := store.OpenWriter(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.InsertObs(ctx, 456, []model.DeviceObs{{Epoch: initial}}); err != nil {
		t.Fatal(err)
	}
	first, err := syncBatch(ctx, live, writer, 456, now)
	if err == nil || first.Done || first.Resume != initial+10*86400+1 {
		t.Fatalf("partial sync=%+v err=%v", first, err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	history.mu.Lock()
	history.failFrom = 0
	history.mu.Unlock()
	for call := range 2 {
		writer, err = store.OpenWriter(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		result, err := syncBatch(ctx, live, writer, 456, now)
		if err != nil || result.Done != (call == 1) || result.RowsAdded != call {
			t.Fatalf("resumed call%d=%+v err=%v", call, result, err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	windows := history.requests()
	if len(windows) != 15 || windows[3] != windows[2] {
		t.Fatalf("failed interval was not replayed exactly once: %v", windows)
	}
	for i := range windows {
		if i > 0 && i != 3 && windows[i][0] != windows[i-1][1]+1 {
			t.Fatalf("sync skipped or repeated a committed interval: %v", windows)
		}
	}
}

func TestSyncDiscardsCursorAfterWatermarkChanges(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const initial = int64(1700000000)
	history := &requestedHistory{}
	live := batchTestSource(t, history)
	writer, err := store.OpenWriter(ctx, filepath.Join(t.TempDir(), "archive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	if _, err := writer.InsertObs(ctx, 456, []model.DeviceObs{{Epoch: initial + 1}}); err != nil {
		t.Fatal(err)
	}
	state, err := json.Marshal(syncCursor{Watermark: initial, Next: initial + 86400})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.SetMeta(ctx, metaSyncCursor, string(state)); err != nil {
		t.Fatal(err)
	}
	if _, err := syncBatch(ctx, live, writer, 456, initial+2*86400); err != nil {
		t.Fatal(err)
	}
	if windows := history.requests(); len(windows) != 1 || windows[0][0] != initial+2 {
		t.Fatalf("stale cursor skipped unscanned observations: %v", windows)
	}
}

func TestSyncReplaysWindowWhenCheckpointFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const initial = int64(1700000000)
	history := &requestedHistory{}
	live := batchTestSource(t, history)
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	writer, err := store.OpenWriter(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	if _, err := writer.InsertObs(ctx, 456, []model.DeviceObs{{Epoch: initial}}); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, db)
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_sync_meta BEFORE INSERT ON meta WHEN NEW.key='mcp_sync_cursor' BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := syncBatch(ctx, live, writer, 456, initial+86400); err == nil {
		t.Fatal("sync did not report failed cursor persistence")
	}
	if _, present, err := writer.Meta(ctx, metaSyncCursor); err != nil || present {
		t.Fatalf("failed cursor save advanced metadata: present=%v err=%v", present, err)
	}
	if _, err := db.ExecContext(ctx, `DROP TRIGGER fail_sync_meta`); err != nil {
		t.Fatal(err)
	}
	if result, err := syncBatch(ctx, live, writer, 456, initial+86400); err != nil || !result.Done {
		t.Fatalf("sync did not recover after checkpoint failure: %+v err=%v", result, err)
	}
	if windows := history.requests(); len(windows) != 2 || windows[0] != windows[1] {
		t.Fatalf("failed checkpoint skipped an empty interval: %v", windows)
	}
}

func TestSyncCancellationReplaysUncommittedWindow(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const initial = int64(1700000000)
	callCtx, cancelCall := context.WithCancel(ctx)
	defer cancelCall()
	var windows [][2]int64
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, _ := strconv.ParseInt(r.URL.Query().Get("time_start"), 10, 64)
		end, _ := strconv.ParseInt(r.URL.Query().Get("time_end"), 10, 64)
		mu.Lock()
		windows = append(windows, [2]int64{start, end})
		first := len(windows) == 1
		mu.Unlock()
		if first {
			cancelCall()
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeObs(w, nil)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TEMPEST_API_BASE", srv.URL)
	t.Setenv("TEMPEST_THROTTLE_MS", "0")
	client, err := newAPIClient("synthetic-token")
	if err != nil {
		t.Fatal(err)
	}
	live := &liveSource{client: client}
	writer, err := store.OpenWriter(ctx, filepath.Join(t.TempDir(), "archive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	if _, err := writer.InsertObs(ctx, 456, []model.DeviceObs{{Epoch: initial}}); err != nil {
		t.Fatal(err)
	}
	if _, err := syncBatch(callCtx, live, writer, 456, initial+86400); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled sync error=%v", err)
	}
	if _, present, err := writer.Meta(ctx, metaSyncCursor); err != nil || present {
		t.Fatalf("canceled window advanced cursor: present=%v err=%v", present, err)
	}
	if result, err := syncBatch(ctx, live, writer, 456, initial+86400); err != nil || !result.Done {
		t.Fatalf("retry after cancellation=%+v err=%v", result, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(windows) != 2 || windows[0] != windows[1] {
		t.Fatalf("canceled window was not replayed: %v", windows)
	}
}

func TestSyncCompletedScanRetriesLateArrivingObservations(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const initial = int64(1700000000)
	const now = initial + 86400
	history := &requestedHistory{}
	live := batchTestSource(t, history)
	writer, err := store.OpenWriter(ctx, filepath.Join(t.TempDir(), "archive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	if _, err := writer.InsertObs(ctx, 456, []model.DeviceObs{{Epoch: initial}}); err != nil {
		t.Fatal(err)
	}
	if result, err := syncBatch(ctx, live, writer, 456, now); err != nil || !result.Done {
		t.Fatalf("first sync=%+v err=%v", result, err)
	}
	history.mu.Lock()
	history.epochs = []int64{now}
	history.mu.Unlock()
	if result, err := syncBatch(ctx, live, writer, 456, now+1); err != nil || !result.Done || result.RowsAdded != 1 {
		t.Fatalf("late-arriving observation was skipped: %+v err=%v", result, err)
	}
}
