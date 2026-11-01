package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/collect"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func runStatus(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "status.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	previous := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = previous }()
	var stderr bytes.Buffer
	code := run(append([]string{"status"}, args...), f, &stderr)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), stderr.String(), code
}

func statusArchive(t *testing.T, epochs []int64, marker string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	w, err := store.OpenWriter(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []model.DeviceObs
	for _, epoch := range epochs {
		rows = append(rows, model.DeviceObs{Epoch: epoch})
	}
	_, insertErr := w.InsertObs(t.Context(), 123, rows)
	var metaErr error
	if marker != "" {
		metaErr = w.SetMeta(t.Context(), collect.MetaBackfillComplete, marker)
	}
	if err := errors.Join(insertErr, metaErr, w.Close()); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestE2EOfflineArchiveStatus(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("TEMPEST_TOKEN", "")
	t.Setenv("TEMPEST_API_BASE", "invalid-must-not-be-used")
	base := int64(1700000000)
	path := statusArchive(t, []int64{base, base + 3600, base + 10801}, "1")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out, stderr, code := runStatus(t, "--db", path, "--format", "json")
	if code != exitOK || stderr != "" {
		t.Fatalf("status=%d stderr=%q output=%q", code, stderr, out)
	}
	var got struct {
		Observations     int64       `json:"observations"`
		FirstObs         *string     `json:"first_obs"`
		LastObs          *string     `json:"last_obs"`
		BackfillComplete bool        `json:"backfill_complete"`
		GapThreshold     int64       `json:"gap_threshold_seconds"`
		GapLimit         int         `json:"gap_limit"`
		Gaps             []store.Gap `json:"gaps"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Observations != 3 || !got.BackfillComplete || got.FirstObs == nil || got.LastObs == nil || got.GapThreshold != 3600 || got.GapLimit != 10 {
		t.Fatalf("incorrect status: %s", out)
	}
	if len(got.Gaps) != 1 || got.Gaps[0].From != base+3600 || got.Gaps[0].To != base+10801 || got.Gaps[0].Seconds != 7201 {
		t.Fatalf("gap threshold/boundaries: %+v", got.Gaps)
	}
	out, stderr, code = runStatus(t, "--db", path)
	if code != exitOK || stderr != "" || !strings.Contains(out, "7201 seconds") || !strings.Contains(out, "Backfill complete: yes") || !strings.Contains(out, "continuous coverage") {
		t.Fatalf("text status=%d stderr=%q output=%q", code, stderr, out)
	}
	if strings.Contains(out, "Archive is empty") || strings.Contains(out, "No qualifying internal gaps") || !strings.Contains(out, "First: "+*got.FirstObs) || !strings.Contains(out, "Last: "+*got.LastObs) {
		t.Fatalf("populated status hides timestamps or denies its gaps: %s", out)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("read-only status changed archive: %v", err)
	}
}

func TestE2EEmptyArchiveStatus(t *testing.T) {
	t.Chdir(t.TempDir())
	path := statusArchive(t, nil, "")
	out, stderr, code := runStatus(t, "--db", path, "--format", "json")
	if code != exitOK || stderr != "" {
		t.Fatalf("status=%d stderr=%q", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"first_obs", "last_obs", "last_obs_age_seconds"} {
		if value, ok := got[key]; !ok || value != nil {
			t.Errorf("empty %s=%v exists=%v", key, value, ok)
		}
	}
	gaps, ok := got["gaps"].([]any)
	if !ok || len(gaps) != 0 || got["observations"] != float64(0) || got["fresh"] != false {
		t.Fatalf("empty status: %s", out)
	}
	out, stderr, code = runStatus(t, "--db", path)
	if code != exitOK || stderr != "" || !strings.Contains(out, "Archive is empty") || !strings.Contains(out, "No qualifying internal gaps") || strings.Contains(out, "Newest age:") {
		t.Fatalf("empty text status=%d stderr=%q output=%q", code, stderr, out)
	}
}

func TestStatusRejectsInvalidFormatBeforeIO(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".env", []byte("invalid dotenv line"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "missing", "archive.sqlite")
	out, stderr, code := runStatus(t, "--db", path, "--format", "yaml")
	if code != exitUsage || out != "" || !strings.Contains(stderr, "--format") {
		t.Fatalf("status=%d out=%q stderr=%q", code, out, stderr)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("status created path: %v", err)
	}
}

func TestStatusMissingArchiveDoesNotCreate(t *testing.T) {
	t.Chdir(t.TempDir())
	path := filepath.Join(t.TempDir(), "missing", "archive.sqlite")
	out, stderr, code := runStatus(t, "--db", path)
	if code != exitFailure || out != "" || stderr == "" {
		t.Fatalf("status=%d out=%q stderr=%q", code, out, stderr)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("status created path: %v", err)
	}
}

func TestArchiveStatusFreshnessAndErrors(t *testing.T) {
	base := int64(1700000000)
	path := statusArchive(t, []int64{base}, "0")
	st, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, st)
	for _, age := range []int64{-1, 0, 3599, 3600, 7200} {
		r, err := readArchiveStatus(t.Context(), st, time.Unix(base+age, 0))
		if err != nil || r.LastObsAgeSeconds == nil || *r.LastObsAgeSeconds != age || r.Fresh != (age >= 0 && age < 3600) {
			t.Fatalf("age=%d report=%+v err=%v", age, r, err)
		}
		for _, format := range []string{"text", "json"} {
			if err := writeArchiveStatus(failedOutput{}, r, format); !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("%s output error = %v", format, err)
			}
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := readArchiveStatus(ctx, st, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readArchiveStatus(t.Context(), st, time.Now()); !errors.Is(err, store.ErrClosed) {
		t.Fatalf("closed = %v", err)
	}
	bad, err := store.Open(t.Context(), statusArchive(t, nil, "invalid"))
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, bad)
	if _, err := readArchiveStatus(t.Context(), bad, time.Now()); !errors.Is(err, collect.ErrInvalidCheckpoint) {
		t.Fatalf("completion error = %v", err)
	}
}
