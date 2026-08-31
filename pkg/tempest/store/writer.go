package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

// Writer creates the schema and appends observations. InsertObs is idempotent on
// (device_id, epoch). The zero value is closed. Close is idempotent. Do not run
// two writers against one archive.
type Writer struct {
	db        *sql.DB
	closeOnce sync.Once
	closed    atomic.Bool
	closeErr  error
}

const archiveDeviceIDKey = "archive_device_id"

// OpenWriter opens an archive for writes. It creates the parent directory, file,
// and schema when absent. The caller owns the returned Writer.
func OpenWriter(ctx context.Context, path string) (*Writer, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, archiveFailure("create archive directory", err)
		}
	}
	created, err := prepareArchiveFile(path)
	if err != nil {
		return nil, err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, archiveFailure("inspect prepared archive", err)
	}
	db, err := sql.Open("sqlite", dsn(path, false,
		"journal_mode(WAL)", "busy_timeout(5000)", "synchronous(NORMAL)"))
	if err != nil {
		return nil, errors.Join(archiveFailure("initialize archive writer", err), cleanupOwnedArchive(path, created, pathInfo))
	}
	db.SetMaxOpenConns(1) // single writer
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(
			archiveFailure("open archive writer", err),
			archiveFailure("close archive writer", db.Close()),
			cleanupOwnedArchive(path, created, pathInfo),
		)
	}
	if err := verifyArchiveIdentity(path, pathInfo); err != nil {
		return nil, errors.Join(err, archiveFailure("close archive writer", db.Close()))
	}
	// schema.sql defines the shared obs_st archive schema; see the file itself.
	if _, err := db.ExecContext(ctx, qrySchema); err != nil {
		return nil, errors.Join(
			archiveFailure("create archive schema", err),
			archiveFailure("close archive writer", db.Close()),
			cleanupOwnedArchive(path, created, pathInfo),
		)
	}
	if err := verifyArchiveIdentity(path, pathInfo); err != nil {
		return nil, errors.Join(err, archiveFailure("close archive writer", db.Close()))
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, errors.Join(
			archiveFailure("restrict archive permissions", err),
			archiveFailure("close archive writer", db.Close()),
			cleanupOwnedArchive(path, created, pathInfo),
		)
	}
	if err := verifyArchiveIdentity(path, pathInfo); err != nil {
		return nil, errors.Join(err, archiveFailure("close archive writer", db.Close()))
	}
	return &Writer{db: db}, nil
}

func verifyArchiveIdentity(path string, expected os.FileInfo) error {
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(expected, current) {
		return fmt.Errorf("%w: archive changed while it was opened", ErrInvalidArchive)
	}
	return nil
}

func cleanupNewArchive(path string, created bool) error {
	if !created {
		return nil
	}
	var errs []error
	for _, candidate := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		if err := os.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, archiveFailure("remove incomplete archive", err))
		}
	}
	return errors.Join(errs...)
}

func cleanupOwnedArchive(path string, created bool, expected os.FileInfo) error {
	if !created {
		return nil
	}
	if err := verifyArchiveIdentity(path, expected); err != nil {
		return err
	}
	return cleanupNewArchive(path, true)
}

func prepareArchiveFile(path string) (bool, error) {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		file, createErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if createErr != nil {
			return false, archiveFailure("create archive", createErr)
		}
		createdInfo, statErr := file.Stat()
		closeErr := file.Close()
		if statErr != nil || closeErr != nil {
			var cleanupErr error
			if statErr == nil {
				cleanupErr = cleanupOwnedArchive(path, true, createdInfo)
			}
			return false, errors.Join(
				archiveFailure("inspect new archive", statErr),
				archiveFailure("close new archive", closeErr),
				cleanupErr,
			)
		}
		return true, nil
	case err != nil:
		return false, archiveFailure("inspect archive", err)
	case info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular():
		return false, fmt.Errorf("%w: archive path must be a regular file, not a symlink", ErrInvalidArgument)
	default:
		if err := os.Chmod(path, 0o600); err != nil {
			return false, archiveFailure("restrict archive permissions", err)
		}
		return false, nil
	}
}

func (w *Writer) database() (*sql.DB, error) {
	if w == nil || w.db == nil || w.closed.Load() {
		return nil, ErrClosed
	}
	return w.db, nil
}

// Close releases the database handle. Repeated calls return the first result.
func (w *Writer) Close() error {
	if w == nil {
		return nil
	}
	w.closeOnce.Do(func() {
		if w.db != nil {
			w.closed.Store(true)
			w.closeErr = archiveFailure("close archive writer", w.db.Close())
		}
	})
	return w.closeErr
}

// BindDevice permanently associates an archive with one Tempest device. Every
// aggregate read spans the archive, so silently inserting a second device would
// double-count rain and mix unrelated temperatures and wind. Legacy archives
// without the metadata key are adopted when their existing rows all match.
func (w *Writer) BindDevice(ctx context.Context, deviceID int) error {
	if deviceID <= 0 {
		return fmt.Errorf("%w: device id must be positive", ErrInvalidArgument)
	}
	db, err := w.database()
	if err != nil {
		return err
	}
	ids, err := readDeviceIDs(ctx, db)
	if err != nil {
		return archiveFailure("read archive devices", err)
	}
	if len(ids) > 1 {
		return fmt.Errorf("%w: archive contains multiple device IDs", ErrInvalidArchive)
	}
	if len(ids) == 1 && ids[0] != deviceID {
		return fmt.Errorf("%w: archive belongs to a different device", ErrDeviceMismatch)
	}

	if raw, ok, err := w.Meta(ctx, archiveDeviceIDKey); err != nil {
		return err
	} else if ok {
		return validateBoundDevice(raw, deviceID)
	}

	// Claim the archive without overwriting a claim another writer may have
	// made after our read. SQLite serializes this statement across processes;
	// INSERT OR IGNORE makes exactly one device win.
	if _, err := db.ExecContext(ctx, qryMetaBindDevice, strconv.Itoa(deviceID)); err != nil {
		return archiveFailure("bind archive device", err)
	}
	raw, ok, err := w.Meta(ctx, archiveDeviceIDKey)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("archive device binding was not persisted")
	}
	return validateBoundDevice(raw, deviceID)
}

func validateBoundDevice(raw string, deviceID int) error {
	bound, err := strconv.Atoi(raw)
	if err != nil || bound <= 0 {
		return fmt.Errorf("%w: archive has invalid device metadata", ErrInvalidArchive)
	}
	if bound != deviceID {
		return fmt.Errorf("%w: archive belongs to a different device", ErrDeviceMismatch)
	}
	return nil
}

// Checkpoint flushes the write-ahead log back into the main database file and
// truncates it. Call it before snapshotting the archive so a file copy captures
// every committed row (WAL frames not yet checkpointed would otherwise be missed).
func (w *Writer) Checkpoint(ctx context.Context) error {
	db, err := w.database()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return archiveFailure("checkpoint archive", err)
}

// InsertObs appends observations for a device in a single transaction and
// returns how many rows were newly inserted; duplicates are ignored, not counted.
// Invalid rows fail the batch before the archive is bound or a transaction is
// opened. A nil *float64 field stores as SQL NULL.
func (w *Writer) InsertObs(ctx context.Context, deviceID int, obs []model.DeviceObs) (added int, err error) {
	if deviceID <= 0 {
		return 0, fmt.Errorf("%w: device id must be positive", ErrInvalidArgument)
	}
	if len(obs) == 0 {
		return 0, nil
	}
	if len(obs) > MaxInsertBatch {
		return 0, fmt.Errorf("%w: observation batch exceeds %d rows", ErrInvalidArgument, MaxInsertBatch)
	}
	for index, observation := range obs {
		if err := observation.Validate(); err != nil {
			return 0, fmt.Errorf("%w: observation %d: %w", ErrInvalidArgument, index, err)
		}
	}
	if err := w.BindDevice(ctx, deviceID); err != nil {
		return 0, err
	}
	db, err := w.database()
	if err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, archiveFailure("begin observation insert", err)
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, archiveFailure("roll back observation insert", tx.Rollback()))
		}
	}()
	stmt, err := tx.PrepareContext(ctx, qryInsertObs)
	if err != nil {
		return 0, archiveFailure("prepare observation insert", err)
	}
	defer func() { err = errors.Join(err, archiveFailure("close observation insert", stmt.Close())) }()

	for _, o := range obs {
		res, err := stmt.ExecContext(ctx, deviceID, o.Epoch,
			o.WindLull, o.WindAvg, o.WindGust, o.WindDir, o.WindInterval,
			o.PressureMb, o.AirTempC, o.Humidity, o.IlluminanceLux, o.UV, o.SolarWm2,
			o.RainMm, o.PrecipType, o.StrikeDistKm, o.StrikeCount, o.BatteryV,
			o.ReportIntervalMin)
		if err != nil {
			return 0, archiveFailure("insert observation", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, archiveFailure("count inserted observations", err)
		}
		added += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, archiveFailure("commit observations", err)
	}
	committed = true
	return added, nil
}

// Watermark returns the newest stored epoch for a device and whether any rows
// exist for it. An incremental sync fetches only observations after this, so it
// stays cheap to run often.
func (w *Writer) Watermark(ctx context.Context, deviceID int) (int64, bool, error) {
	if deviceID <= 0 {
		return 0, false, fmt.Errorf("%w: device id must be positive", ErrInvalidArgument)
	}
	db, err := w.database()
	if err != nil {
		return 0, false, err
	}
	var newest sql.NullInt64
	err = db.QueryRowContext(ctx, qryWatermark, deviceID).Scan(&newest)
	if err != nil {
		return 0, false, archiveFailure("read archive watermark", err)
	}
	return newest.Int64, newest.Valid, nil
}

// Coverage reports the row count and epoch span stored for a device.
func (w *Writer) Coverage(ctx context.Context, deviceID int) (Coverage, error) {
	var c Coverage
	if deviceID <= 0 {
		return c, fmt.Errorf("%w: device id must be positive", ErrInvalidArgument)
	}
	db, err := w.database()
	if err != nil {
		return c, err
	}
	err = db.QueryRowContext(ctx, qryWriterCoverage, deviceID).
		Scan(&c.Count, &c.MinEpoch, &c.MaxEpoch)
	return c, archiveFailure("read writer coverage", err)
}

// Meta reads a value from the archive's key/value meta table, reporting whether
// the key was present. It backs small pieces of collector state (such as the
// backward-backfill cursor) that let a long, chunked backfill resume across
// separate runs or tool calls.
func (w *Writer) Meta(ctx context.Context, key string) (string, bool, error) {
	if err := validateMetadataKey(key); err != nil {
		return "", false, err
	}
	db, err := w.database()
	if err != nil {
		return "", false, err
	}
	var v string
	switch err := db.QueryRowContext(ctx, qryMetaGet, key).Scan(&v); {
	case err == nil:
		return v, true, nil
	case errors.Is(err, sql.ErrNoRows):
		return "", false, nil
	default:
		return "", false, archiveFailure("read writer metadata", err)
	}
}

// SetMeta upserts a key/value pair into the meta table.
func (w *Writer) SetMeta(ctx context.Context, key, value string) error {
	if err := validateMetadataKey(key); err != nil {
		return err
	}
	if len(value) > MaxMetadataValueBytes || strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%w: metadata value exceeds %d bytes or contains NUL", ErrInvalidArgument, MaxMetadataValueBytes)
	}
	db, err := w.database()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, qryMetaSet, key, value)
	return archiveFailure("write archive metadata", err)
}
