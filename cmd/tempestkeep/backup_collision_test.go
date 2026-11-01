package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSnapshotSameClockPreservesEachCopy(t *testing.T) {
	src := filepath.Join(t.TempDir(), "weather.sqlite")
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	seen := map[string]string{}
	for i := range 3 {
		content := fmt.Sprintf("synthetic snapshot %d", i)
		if err := os.WriteFile(src, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		dest, err := snapshotAt(src, 7, now)
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := seen[dest]; exists {
			t.Fatalf("reused snapshot %q", dest)
		}
		seen[dest] = content
		if got, err := os.ReadFile(dest); err != nil || string(got) != content {
			t.Fatalf("snapshot=%q, error=%v", got, err)
		}
	}
	for dest, content := range seen {
		if got, err := os.ReadFile(dest); err != nil || string(got) != content {
			t.Fatalf("prior snapshot changed: %q, %v", got, err)
		}
	}
	if _, err := snapshotAt(src, 2, now); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(filepath.Dir(src), "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("retained entries=%d, error=%v", len(entries), err)
	}
	assertNoBackupTemps(t, backupDir)
}

func assertNoBackupTemps(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Errorf("temporary file remains: %s", entry.Name())
		}
	}
}

func TestPruneMixedBackupFormatsByInstant(t *testing.T) {
	dir := t.TempDir()
	names := []string{"weather-20261101-120000.sqlite", "weather-20261101-120000.000000000.sqlite", "weather-20261101-120000.000000001.sqlite", "weather-20261101-120001.sqlite"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	unrelated := "weather-20261101-120000.1.sqlite"
	if err := os.WriteFile(filepath.Join(dir, unrelated), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := pruneBackups(dir, "weather", 2); err != nil {
		t.Fatal(err)
	}
	for _, name := range names[:2] {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("old snapshot remains: %s (%v)", name, err)
		}
	}
	for _, name := range append(names[2:], unrelated) {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("preserved snapshot %s: %v", name, err)
		}
	}
}

func TestSnapshotCollisionLimitCleansTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "weather.sqlite")
	backupDir := filepath.Join(dir, "backups")
	if err := os.WriteFile(src, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(backupDir, 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for i := range 100 {
		name := "weather-" + now.Add(time.Duration(i)).Format("20060102-150405.000000000") + ".sqlite"
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := snapshotAt(src, 7, now); err == nil {
		t.Fatal("expected bounded collision error")
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 100 {
		t.Fatalf("entries=%d, expected 100", len(entries))
	}
	for _, entry := range entries {
		if got, err := os.ReadFile(filepath.Join(backupDir, entry.Name())); err != nil || string(got) != "old" {
			t.Fatalf("existing backup changed: %q, %v", got, err)
		}
	}
	assertNoBackupTemps(t, backupDir)
}
