package store_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	_ "modernc.org/sqlite"
)

const writeDevice = 456

func TestWriterInsertIdempotentAndReadParity(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "archive.sqlite")

	w, err := store.OpenWriter(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}

	obs := []model.DeviceObs{
		{Epoch: 1700000000, AirTempC: new(float64(10)), WindGust: new(float64(3)), PrecipType: new(float64(0))},
		{Epoch: 1700000060, AirTempC: new(float64(15)), WindGust: new(float64(8)), PrecipType: new(float64(1))},
		{Epoch: 1700000120, AirTempC: new(float64(20)), WindGust: new(float64(12)), PrecipType: new(float64(0))},
	}
	added, err := w.InsertObs(ctx, writeDevice, obs)
	if err != nil {
		t.Fatalf("InsertObs: %v", err)
	}
	if added != 3 {
		t.Errorf("added = %d, want 3", added)
	}

	// Watermark is the newest stored epoch.
	wm, ok, err := w.Watermark(ctx, writeDevice)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || wm != 1700000120 {
		t.Errorf("watermark = %d (ok=%v), want 1700000120", wm, ok)
	}

	// Coverage spans the inserted rows.
	cov, err := w.Coverage(ctx, writeDevice)
	if err != nil {
		t.Fatal(err)
	}
	if cov.Count != 3 || cov.MinEpoch.Int64 != 1700000000 || cov.MaxEpoch.Int64 != 1700000120 {
		t.Errorf("coverage = %+v, want count 3, min 1700000000, max 1700000120", cov)
	}

	// Re-inserting the identical rows adds nothing (INSERT OR IGNORE).
	again, err := w.InsertObs(ctx, writeDevice, obs)
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Errorf("re-insert added = %d, want 0", again)
	}

	// Reject the whole batch when one row is invalid. The valid row must not be
	// committed before validation fails.
	mixed := []model.DeviceObs{
		{Epoch: 1700000120, AirTempC: new(float64(99))}, // duplicate; temp 20 must survive
		{Epoch: 1700000180, AirTempC: new(float64(25))}, // new
		{Epoch: 0, AirTempC: new(float64(1))},           // invalid
	}
	n, err := w.InsertObs(ctx, writeDevice, mixed)
	if !errors.Is(err, store.ErrInvalidArgument) {
		t.Fatalf("InsertObs error = %v, want ErrInvalidArgument", err)
	}
	if n != 0 {
		t.Errorf("mixed insert added = %d, want 0", n)
	}
	n, err = w.InsertObs(ctx, writeDevice, mixed[1:2])
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("valid insert added = %d, want 1", n)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Parity: the read-only Store opens the same file (which requires the obs_st
	// table) and reads back exactly what the Writer wrote.
	s, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("store.Open after write: %v", err)
	}
	closeOnCleanup(t, s)

	latest, err := s.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.Epoch != 1700000180 || latest.AirTempC == nil || *latest.AirTempC != 25 {
		t.Errorf("latest = %+v, want epoch 1700000180 temp 25", latest)
	}

	cov2, err := s.Coverage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cov2.Count != 4 {
		t.Errorf("final count = %d, want 4 (3 + 1 new; dup and epoch<=0 excluded)", cov2.Count)
	}

	// The duplicate must not have overwritten temp 20 with 99: the hottest
	// reading is still 25°C, which also proves 99 was never inserted.
	r, err := s.Records(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.HottestF == nil || !almost(*r.HottestF, model.CToF(25)) {
		t.Errorf("hottest = %v, want %v (dup epoch must not overwrite)", r.HottestF, model.CToF(25))
	}
}

func TestOpenWriterEnablesWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.sqlite")
	w, err := store.OpenWriter(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	// Close first so the check reads the persistent on-disk mode, not a live
	// second connection (WAL mode survives in the file header after close).
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, db)
	var mode string
	if err := db.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

func TestWriterRestrictsFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.sqlite")
	w, err := store.OpenWriter(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, w)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("archive mode = %o, want 600", perm)
		}
	}
}

func TestWriterRejectsSecondDevice(t *testing.T) {
	w, err := store.OpenWriter(context.Background(), filepath.Join(t.TempDir(), "one-device.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, w)
	ctx := context.Background()
	if _, err := w.InsertObs(ctx, 1, []model.DeviceObs{{Epoch: 100}}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.InsertObs(ctx, 2, []model.DeviceObs{{Epoch: 200}}); !errors.Is(err, store.ErrDeviceMismatch) {
		t.Fatalf("second device error = %v, want archive ownership error", err)
	}
}

func TestConcurrentWritersCannotClaimDifferentDevices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raced-device-claim.sqlite")
	w1, err := store.OpenWriter(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, w1)
	w2, err := store.OpenWriter(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, w2)

	start := make(chan struct{})
	errs := make(chan error, 2)
	insert := func(w *store.Writer, deviceID int) {
		<-start
		_, err := w.InsertObs(context.Background(), deviceID, []model.DeviceObs{{Epoch: int64(100 + deviceID)}})
		errs <- err
	}
	go insert(w1, 1)
	go insert(w2, 2)
	close(start)

	succeeded := 0
	failed := 0
	for range 2 {
		if err := <-errs; err != nil {
			if !errors.Is(err, store.ErrDeviceMismatch) {
				t.Fatalf("losing writer returned unexpected error: %v", err)
			}
			failed++
		} else {
			succeeded++
		}
	}
	if succeeded != 1 || failed != 1 {
		t.Fatalf("concurrent claims: %d succeeded, %d failed; want one each", succeeded, failed)
	}
}

func TestArchivePathEscapesURIMetacharacters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "weather?#archive.sqlite")
	w, err := store.OpenWriter(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if _, err := w.InsertObs(context.Background(), writeDevice, []model.DeviceObs{{Epoch: 100}}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the exact requested filename was not created: %v", err)
	}
	s, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	closeOnCleanup(t, s)
}

func TestWriterMetaAndCheckpoint(t *testing.T) {
	ctx := context.Background()
	w, err := store.OpenWriter(context.Background(), filepath.Join(t.TempDir(), "meta.sqlite"))
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	closeOnCleanup(t, w)

	// A missing key reports not-present, not an error.
	if _, ok, err := w.Meta(ctx, "backfill_cursor"); err != nil || ok {
		t.Errorf("Meta(missing) = ok %v err %v, want false/nil", ok, err)
	}
	if err := w.SetMeta(ctx, "backfill_cursor", "1700000000"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	if v, ok, err := w.Meta(ctx, "backfill_cursor"); err != nil || !ok || v != "1700000000" {
		t.Errorf("Meta = %q ok %v err %v, want 1700000000", v, ok, err)
	}
	// Upsert overwrites in place.
	if err := w.SetMeta(ctx, "backfill_cursor", "1699000000"); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := w.Meta(ctx, "backfill_cursor"); v != "1699000000" {
		t.Errorf("Meta after upsert = %q, want 1699000000", v)
	}
	// Checkpoint is a no-op-safe flush.
	if err := w.Checkpoint(ctx); err != nil {
		t.Errorf("Checkpoint: %v", err)
	}
}

func TestGaps(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "gaps.sqlite")
	w, err := store.OpenWriter(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	// Two tight clusters (60s spacing) separated by a large hole.
	obs := []model.DeviceObs{
		{Epoch: 1000, AirTempC: new(float64(10))},
		{Epoch: 1060, AirTempC: new(float64(10))},
		{Epoch: 1120, AirTempC: new(float64(10))},
		{Epoch: 50000, AirTempC: new(float64(10))}, // ~48800s gap after 1120
		{Epoch: 50060, AirTempC: new(float64(10))},
	}
	if _, err := w.InsertObs(ctx, writeDevice, obs); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	closeOnCleanup(t, s)

	gaps, err := s.Gaps(ctx, 3600, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 {
		t.Fatalf("gaps = %d (%+v), want exactly 1 (only the big hole exceeds 1h)", len(gaps), gaps)
	}
	if gaps[0].From != 1120 || gaps[0].To != 50000 || gaps[0].Seconds != 48880 {
		t.Errorf("gap = %+v, want 1120->50000 (48880s)", gaps[0])
	}
	if gaps[0].FromLocal == "" || gaps[0].ToLocal == "" {
		t.Error("gap is missing human-readable timestamps")
	}
}

func TestInsertObsEmpty(t *testing.T) {
	w, err := store.OpenWriter(context.Background(), filepath.Join(t.TempDir(), "empty.sqlite"))
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	closeOnCleanup(t, w)
	n, err := w.InsertObs(context.Background(), writeDevice, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("added = %d, want 0 for an empty batch", n)
	}
	if _, err := w.InsertObs(t.Context(), 0, nil); !errors.Is(err, store.ErrInvalidArgument) {
		t.Fatalf("invalid device error = %v, want ErrInvalidArgument", err)
	}
}

func TestOpenWriterRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.sqlite")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "archive.sqlite")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := store.OpenWriter(t.Context(), link); !errors.Is(err, store.ErrInvalidArgument) {
		t.Fatalf("OpenWriter error = %v, want ErrInvalidArgument", err)
	}
}
