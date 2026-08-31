package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSnapshotCopiesContentWithPrivatePermissions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "weather.sqlite")
	if err := os.WriteFile(src, []byte("archive bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst, err := snapshot(src, 2)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "archive bytes" {
		t.Fatalf("snapshot content = %q", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(dst)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("snapshot mode = %o, want 600", perm)
		}
		backupDir, err := os.Stat(filepath.Dir(dst))
		if err != nil {
			t.Fatal(err)
		}
		if perm := backupDir.Mode().Perm(); perm != 0o700 {
			t.Fatalf("backup directory mode = %o, want 700", perm)
		}
	}
}

func TestSnapshotNeverOverwritesExistingBackup(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "weather.sqlite")
	destination := filepath.Join(dir, "existing.sqlite")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(source, destination); err == nil {
		t.Fatal("copyFile overwrote an existing destination")
	}
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "old" {
		t.Fatalf("existing backup changed to %q", contents)
	}
}

func TestCopyFileRejectsSourceSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions differ on Windows")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "weather.sqlite")
	link := filepath.Join(dir, "weather-link.sqlite")
	if err := os.WriteFile(source, []byte("archive bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(link, filepath.Join(dir, "backup.sqlite")); err == nil {
		t.Fatal("copyFile accepted a source symlink")
	}
}

func TestSnapshotRejectsBackupDirectorySymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions differ on Windows")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "weather.sqlite")
	outside := t.TempDir()
	if err := os.WriteFile(source, []byte("archive bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "backups")); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot(source, 2); err == nil {
		t.Fatal("snapshot accepted a backup directory symlink")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("snapshot wrote %d file(s) through a directory symlink", len(entries))
	}
}

func TestPruneBackupsOnlyRemovesOwnedSnapshots(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"weather-20260827-120000.sqlite",
		"weather-20260828-120000.sqlite",
		"weather-20260829-120000.sqlite",
		"weather-not-a-snapshot.sqlite",
		"other-20260820-120000.sqlite",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneBackups(dir, "weather", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, files[0])); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old snapshot stat error = %v, want not exist", err)
	}
	for _, name := range files[1:] {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("preserved file %q: %v", name, err)
		}
	}
}

func TestPruneBackupsRejectsInvalidRetention(t *testing.T) {
	for _, keep := range []int{-1, 0, maxBackupKeep + 1} {
		if err := pruneBackups(t.TempDir(), "weather", keep); err == nil {
			t.Errorf("keep %d: expected an error", keep)
		}
	}
}
