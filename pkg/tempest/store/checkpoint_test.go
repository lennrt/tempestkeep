package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

func TestCheckpointRejectsBusyReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	writer, err := OpenWriter(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Errorf("close writer: %v", err)
		}
	})
	// Keep the test fast without changing the production lock timeout.
	if _, err := writer.db.ExecContext(t.Context(), "PRAGMA busy_timeout(1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.InsertObs(t.Context(), 1, []model.DeviceObs{{Epoch: 100}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Checkpoint(t.Context()); err != nil {
		t.Fatal(err)
	}

	reader, err := sql.Open("sqlite", dsn(path, true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Errorf("close reader: %v", err)
		}
	})
	tx, err := reader.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Errorf("roll back reader: %v", err)
		}
	})
	var count int
	if err := tx.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM obs_st").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("initial count = %d, want 1", count)
	}
	if _, err := writer.InsertObs(t.Context(), 1, []model.DeviceObs{{Epoch: 200}}); err != nil {
		t.Fatal(err)
	}
	// The reader pins the earlier snapshot. SQLite returns busy=1 in the
	// PRAGMA result, without an SQL error, and leaves the new row in the WAL.
	if err := writer.Checkpoint(t.Context()); !errors.Is(err, ErrArchiveIO) {
		t.Fatalf("checkpoint with pinned reader = %v, want ErrArchiveIO", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Checkpoint(t.Context()); err != nil {
		t.Fatalf("checkpoint after reader release: %v", err)
	}
	var busy, remaining, checkpointed int
	if err := writer.db.QueryRowContext(t.Context(), "PRAGMA wal_checkpoint(PASSIVE)").Scan(&busy, &remaining, &checkpointed); err != nil {
		t.Fatal(err)
	}
	if busy != 0 || remaining != 0 || checkpointed != 0 {
		t.Fatalf("WAL after truncate: busy=%d remaining=%d checkpointed=%d", busy, remaining, checkpointed)
	}
}
