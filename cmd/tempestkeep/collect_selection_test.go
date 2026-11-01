package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"

	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func TestCollectExplicitZeroBeforeConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".env", []byte("invalid dotenv\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "absent.sqlite")
	if err := cmdCollect([]string{"--db", path, "--device-id=0"}); !isUsageErr(err) {
		t.Fatalf("error=%v, want usage", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created archive: %v", err)
	}
}

func TestCollectPreservesArchiveDeviceDespiteStationOrder(t *testing.T) {
	t.Chdir(t.TempDir())
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	w, err := store.OpenWriter(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.BindDevice(t.Context(), 456); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var discovery, observations atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/stations") {
			discovery.Add(1)
			if _, err := fmt.Fprint(w, `{"stations":[{"station_id":1,"devices":[{"device_id":789,"device_type":"ST"}]}]}`); err != nil {
				t.Error(err)
			}
			return
		}
		observations.Add(1)
		if !strings.HasSuffix(r.URL.Path, "/observations/device/456") {
			t.Errorf("unexpected observation path %s", r.URL.Path)
		}
		if _, err := fmt.Fprint(w, `{"obs":[]}`); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	configureCollectionRangeTest(t, srv.URL)
	t.Setenv("TEMPEST_DEVICE_ID", "")
	args := []string{"--db", path, "--start=2025-01-01", "--end=2025-01-01", "--no-backup", "--quiet"}
	if err := cmdCollect(args); err != nil {
		t.Fatalf("collect existing archive: %v", err)
	}
	if discovery.Load() != 0 || observations.Load() == 0 {
		t.Fatalf("discovery=%d observations=%d", discovery.Load(), observations.Load())
	}
	// Explicit selection still wins, and writer binding rejects it before HTTP.
	n := observations.Load()
	if err := cmdCollect(append(args, "--device-id=789")); !errors.Is(err, store.ErrDeviceMismatch) {
		t.Fatalf("mismatch=%v", err)
	}
	if observations.Load() != n {
		t.Fatal("mismatched explicit selector made observation requests")
	}
}

func TestCollectionDeviceResolution(t *testing.T) {
	for _, tc := range []struct {
		name, setup string
		want        int
		fail        bool
	}{
		{"missing", "missing", 0, false},
		{"empty unbound", "empty", 0, false},
		{"empty bound", "bound", 456, false},
		{"legacy rows", "legacy", 456, false},
		{"unidentifiable legacy", "unknown", 0, true},
		{"invalid metadata", "invalid", 0, true},
		{"oversized metadata", "oversized", 0, true},
		{"no metadata table", "no-meta", 456, false},
		{"mixed rows", "mixed", 0, true},
		{"corrupt file", "corrupt", 0, true},
		{"directory", "directory", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "archive.sqlite")
			switch tc.setup {
			case "missing":
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.WriteFile(path, []byte("not sqlite"), 0600); err != nil {
					t.Fatal(err)
				}
			default:
				w, err := store.OpenWriter(t.Context(), path)
				if err != nil {
					t.Fatal(err)
				}
				if tc.setup != "empty" {
					if _, err := w.InsertObs(t.Context(), 456, []model.DeviceObs{{Epoch: 1700000000}}); err != nil {
						t.Fatal(err)
					}
				}
				if err := w.Close(); err != nil {
					t.Fatal(err)
				}
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				switch tc.setup {
				case "bound":
					_, err = db.ExecContext(t.Context(), "DELETE FROM obs_st")
				case "legacy":
					_, err = db.ExecContext(t.Context(), "DELETE FROM meta WHERE key='archive_device_id'")
				case "unknown":
					_, err = db.ExecContext(t.Context(), "DELETE FROM meta WHERE key='archive_device_id'")
					if err == nil {
						_, err = db.ExecContext(t.Context(), "ALTER TABLE obs_st RENAME COLUMN device_id TO legacy_device")
					}
				case "oversized":
					_, err = db.ExecContext(t.Context(), "UPDATE meta SET value=? WHERE key='archive_device_id'", strings.Repeat("9", 4097))
				case "no-meta":
					_, err = db.ExecContext(t.Context(), "DROP TABLE meta")
				case "invalid":
					_, err = db.ExecContext(t.Context(), "UPDATE meta SET value='0' WHERE key='archive_device_id'")
				case "mixed":
					_, err = db.ExecContext(t.Context(), "INSERT INTO obs_st (device_id,epoch) VALUES (789,1700000060)")
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}
			got, err := collectionDevice(t.Context(), 0, "", path)
			if (err != nil) != tc.fail || (!tc.fail && got != tc.want) {
				t.Fatalf("device=%d error=%v; want %d fail=%v", got, err, tc.want, tc.fail)
			}
			if tc.setup == "missing" {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("selection created file: %v", err)
				}
			}
			// Explicit selectors must not inspect even malformed archives or environments.
			if got, err := collectionDevice(t.Context(), 123, "bad", path); err != nil || got != 123 {
				t.Fatalf("explicit precedence: %d %v", got, err)
			}
			if got, err := collectionDevice(t.Context(), 0, "789", path); err != nil || got != 789 {
				t.Fatalf("environment precedence: %d %v", got, err)
			}
		})
	}
	for _, value := range []string{"0", "-1", "bad"} {
		if _, err := collectionDevice(t.Context(), 0, value, "unused"); err == nil {
			t.Errorf("accepted environment %q", value)
		}
	}
}

func TestCollectFreshArchiveDiscovers(t *testing.T) {
	t.Chdir(t.TempDir())
	var discovery, observations atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/stations") {
			discovery.Add(1)
			if _, err := fmt.Fprint(w, `{"stations":[{"station_id":1,"devices":[{"device_id":456,"device_type":"ST"}]}]}`); err != nil {
				t.Error(err)
			}
			return
		}
		observations.Add(1)
		if _, err := fmt.Fprint(w, `{"obs":[]}`); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	configureCollectionRangeTest(t, srv.URL)
	t.Setenv("TEMPEST_DEVICE_ID", "")
	path := filepath.Join(t.TempDir(), "fresh.sqlite")
	if err := cmdCollect([]string{"--db", path, "--start=2025-01-01", "--end=2025-01-01", "--no-backup", "--quiet"}); err != nil {
		t.Fatal(err)
	}
	if discovery.Load() != 1 || observations.Load() != 1 {
		t.Fatalf("discovery=%d observations=%d", discovery.Load(), observations.Load())
	}
	got, err := collectionDevice(t.Context(), 0, "", path)
	if err != nil || got != 456 {
		t.Fatalf("new archive identity=%d error=%v", got, err)
	}
}

func TestCollectionDeviceMetadataUsesSharedReader(t *testing.T) {
	for _, tc := range []struct {
		name, encoding, table string
		device                int
	}{
		{"case-insensitive legacy metadata table", "UTF-8", "META", 456},
		{"UTF-16le identity", "UTF-16le", "meta", int(^uint(0) >> 1)},
		{"UTF-16be identity", "UTF-16be", "meta", int(^uint(0) >> 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "archive.sqlite")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, schemaErr := db.ExecContext(t.Context(), fmt.Sprintf("PRAGMA encoding='%s'; CREATE TABLE %s (key TEXT PRIMARY KEY, value TEXT NOT NULL)", tc.encoding, tc.table))
			if err := errors.Join(schemaErr, db.Close()); err != nil {
				t.Fatal(err)
			}
			w, err := store.OpenWriter(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			if err := errors.Join(w.BindDevice(t.Context(), tc.device), w.Close()); err != nil {
				t.Fatal(err)
			}
			got, err := collectionDevice(t.Context(), 0, "", path)
			if err != nil || got != tc.device {
				t.Fatalf("archive device=%d want=%d error=%v", got, tc.device, err)
			}
		})
	}
}
