package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func TestExplicitCollectionRangeCalendarBounds(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, time.December, 3, 12, 34, 56, 0, loc)
	for _, tc := range []struct {
		name, start, end string
		wantHours        int64
		wantError        string
	}{
		{name: "spring forward", start: "2025-03-09", end: "2025-03-09", wantHours: 23},
		{name: "fall back", start: "2025-11-02", end: "2025-11-02", wantHours: 25},
		{name: "366 days", start: "2024-11-03", end: "2025-11-03", wantHours: 366*24 + 1},
		{name: "367 days", start: "2024-11-03", end: "2025-11-04", wantError: "366 calendar days"},
		{name: "missing start", end: "2025-01-01", wantError: "--start"},
		{name: "missing end", start: "2025-01-01", wantError: "--end"},
		{name: "noncanonical", start: "2025-1-01", end: "2025-01-01", wantError: "--start"},
		{name: "reversed", start: "2025-01-02", end: "2025-01-01", wantError: "after --end"},
		{name: "unsupported", start: "1969-12-31", end: "1970-01-01", wantError: "Unix epoch"},
		{name: "future", start: "2025-12-03", end: "2025-12-04", wantError: "future"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end, err := explicitCollectionRange(tc.start, tc.end, now)
			if tc.wantError != "" {
				if !isUsageErr(err) || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want usage error containing %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if end-start+1 != tc.wantHours*3600 {
				t.Fatalf("range has %d seconds, want %d", end-start+1, tc.wantHours*3600)
			}
			if got := time.Unix(start, 0).In(loc).Format(time.DateTime); got != tc.start+" 00:00:00" {
				t.Fatalf("start = %s", got)
			}
			if got := time.Unix(end, 0).In(loc).Format(time.DateTime); got != tc.end+" 23:59:59" {
				t.Fatalf("end = %s", got)
			}
		})
	}
	start, end, err := explicitCollectionRange("2025-12-03", "2025-12-03", now)
	if err != nil || end != now.Unix() || start >= end {
		t.Fatalf("today range = [%d,%d], error=%v; want end=%d", start, end, err, now.Unix())
	}
}

func TestCollectRangeRejectsInvalidInputBeforeIO(t *testing.T) {
	t.Chdir(t.TempDir())
	// Even a broken configuration file must not mask a range usage error.
	if err := os.WriteFile(".env", []byte("not an assignment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TEMPEST_TOKEN", "synthetic-token")
	t.Setenv("TEMPEST_API_BASE", srv.URL)
	path := filepath.Join(t.TempDir(), "must-not-exist.sqlite")
	for _, args := range [][]string{
		{"--start", "2025-01-01"},
		{"--end", "2025-01-01"},
		{"--start=", "--end=2025-01-01"},
		{"--start=2025-01-01", "--end="},
		{"--start=bad", "--end=2025-01-01"},
		{"--start=2025-01-02", "--end=2025-01-01"},
		{"--start=1960-01-01", "--end=1960-01-02"},
		{"--start=2024-01-01", "--end=2025-01-01"},
		{"--start=9999-01-01", "--end=9999-01-01"},
		{"--start=2025-01-01", "--end=2025-01-01", "--backfill-start=2020-01-01"},
		{"--start=2025-01-01", "--end=2025-01-01", "--backfill-start="},
	} {
		if err := cmdCollect(append([]string{"--db", path}, args...)); !isUsageErr(err) {
			t.Errorf("cmdCollect(%q) = %v, want usage error", args, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid ranges made %d requests", requests.Load())
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid range created archive: %v", err)
	}
}

type collectionWindow struct{ start, end int64 }

func TestCollectRangeRepairsGapAndReplaysAfterFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	start, end, err := explicitCollectionRange("2025-01-01", "2025-01-06", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	metadata := seedCollectionRangeArchive(t, path, start-1, end+1)
	windows := make(chan collectionWindow, 10)
	var fail atomic.Bool
	fail.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, _ := strconv.ParseInt(r.URL.Query().Get("time_start"), 10, 64)
		e, _ := strconv.ParseInt(r.URL.Query().Get("time_end"), 10, 64)
		windows <- collectionWindow{s, e}
		if fail.Load() && s > start {
			http.Error(w, "synthetic failure", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"obs":[[%d],[%d]]}`, s, e)
	}))
	t.Cleanup(srv.Close)
	configureCollectionRangeTest(t, srv.URL)
	// An invalid seed floor must not affect explicit range collection.
	t.Setenv("TEMPEST_BACKFILL_START", "invalid-and-irrelevant")
	args := []string{"--db", path, "--device-id=456", "--start=2025-01-01", "--end=2025-01-06", "--quiet"}
	if err := cmdCollect(args); err == nil || !strings.Contains(err.Error(), "same --start and --end") {
		t.Fatalf("partial failure = %v, want range replay guidance", err)
	}
	assertCollectionRangeArchive(t, path, 4, metadata)
	// Failed collection must not create a backup even if the caller normally enables it.
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "backups")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed collection produced backup: %v", err)
	}
	fail.Store(false)
	for range 2 {
		if err := cmdCollect(append(args, "--no-backup")); err != nil {
			t.Fatal(err)
		}
		assertCollectionRangeArchive(t, path, 6, metadata)
	}
	if err := cmdCollect(args); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), "backups", "*.sqlite"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("completed range backups = %v, error=%v", backups, err)
	}
	assertCollectionRangeArchive(t, backups[0], 6, metadata)
	wantWindows := []collectionWindow{{start, start + 5*86400 - 1}, {start + 5*86400, end}}
	for i := range 8 {
		select {
		case got := <-windows:
			if got != wantWindows[i%2] {
				t.Fatalf("request %d = %+v, want %+v", i, got, wantWindows[i%2])
			}
		default:
			t.Fatalf("missing request %d", i)
		}
	}
	if len(windows) != 0 {
		t.Fatalf("unexpected requests beyond explicit range: %d", len(windows))
	}
}

func configureCollectionRangeTest(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("TEMPEST_TOKEN", "synthetic-token")
	t.Setenv("TEMPEST_API_BASE", baseURL)
	t.Setenv("TEMPEST_THROTTLE_MS", "0")
	t.Setenv("TEMPEST_BACKUP_KEEP", "7")
	t.Setenv("TEMPEST_CACHE_TTL", "0")
}

func seedCollectionRangeArchive(t *testing.T, path string, before, after int64) map[string]string {
	t.Helper()
	w, err := store.OpenWriter(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, w)
	if _, err := w.InsertObs(t.Context(), 456, []model.DeviceObs{{Epoch: before}, {Epoch: after}}); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]string{
		"backfill_cursor": "12345", "backfill_complete": "0",
		"mcp_sync_cursor":         `{"watermark":12345,"next":23456}`,
		"mcp_backfill_empty_span": `{"before":12345,"empty_seconds":86400}`,
	}
	for key, value := range metadata {
		if err := w.SetMeta(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return metadata
}

func assertCollectionRangeArchive(t *testing.T, path string, wantRows int64, metadata map[string]string) {
	t.Helper()
	st, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	}()
	cov, err := st.Coverage(t.Context())
	if err != nil || cov.Count != wantRows {
		t.Fatalf("coverage = %+v, error=%v; want %d rows", cov, err, wantRows)
	}
	for key, want := range metadata {
		got, ok, err := st.Meta(t.Context(), key)
		if err != nil || !ok || got != want {
			t.Errorf("metadata %s = %q, exists=%v, error=%v; want %q", key, got, ok, err, want)
		}
	}
}

const collectRangeHelperEnv = "TEMPESTKEEP_TEST_COLLECT_RANGE_HELPER"

func TestCollectRangeSubprocessHelper(t *testing.T) {
	if os.Getenv(collectRangeHelperEnv) != "1" {
		return
	}
	os.Args = []string{"tempestkeep", "collect", "--start=2025-01-01", "--end=2025-01-06", "--device-id=456", "--no-backup", "--quiet"}
	main()
	os.Exit(0)
}

func TestCollectRangeCancellationKeepsCommittedRows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt cannot signal a subprocess on Windows")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.sqlite")
	start, end, err := explicitCollectionRange("2025-01-01", "2025-01-06", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	metadata := seedCollectionRangeArchive(t, path, start-1, end+1)
	secondRequest := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, _ := strconv.ParseInt(r.URL.Query().Get("time_start"), 10, 64)
		if s != start {
			close(secondRequest)
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"obs":[[%d]]}`, s)
	}))
	t.Cleanup(srv.Close)
	configureCollectionRangeTest(t, srv.URL)
	t.Setenv("TEMPEST_DB", path)
	t.Setenv("TEMPEST_BACKFILL_START", "")
	t.Setenv(collectRangeHelperEnv, "1")
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCollectRangeSubprocessHelper$")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	select {
	case <-secondRequest:
	case <-ctx.Done():
		t.Fatal("collector did not reach its second request")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("interrupted collector reported success")
	}
	if got := stderr.String(); !strings.Contains(got, "interrupted") || !strings.Contains(got, "same --start and --end") {
		t.Fatalf("cancellation diagnostic = %q", got)
	}
	assertCollectionRangeArchive(t, path, 3, metadata)
}
