package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/config"
)

func TestWriteEnvFileQuotesValuesAndRestrictsPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("OLD=value\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := writeEnvFile(path,
		map[string]string{"OLD": "value"},
		map[string]string{"TEMPEST_TOKEN": "secret", "TEMPEST_DB": "/tmp/weather archive/#1.sqlite"},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["TEMPEST_TOKEN"] != "secret" || got["TEMPEST_DB"] != "/tmp/weather archive/#1.sqlite" || got["OLD"] != "value" {
		t.Fatalf("readEnvFile() = %#v", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf(".env mode = %o, want 600", perm)
		}
	}
}

func TestCommandArgQuotesSpacesAndQuotes(t *testing.T) {
	got := commandArg(`/tmp/weather archive/owner's.sqlite`)
	if runtime.GOOS == "windows" {
		if got != `"/tmp/weather archive/owner's.sqlite"` {
			t.Fatalf("commandArg() = %q", got)
		}
		return
	}
	if got != `'/tmp/weather archive/owner'"'"'s.sqlite'` {
		t.Fatalf("commandArg() = %q", got)
	}
}

func TestMCPRegistrationCommandWrapsSafely(t *testing.T) {
	lines := mcpRegistrationCommand(`/tmp/weather archive/tempest.sqlite`)
	if runtime.GOOS == "windows" {
		if len(lines) != 1 {
			t.Fatalf("Windows command has %d lines, want 1", len(lines))
		}
		return
	}
	if len(lines) != 3 || lines[0] != `claude mcp add tempest \` ||
		lines[1] != `  -- tempest-mcp \` ||
		lines[2] != `  --db '/tmp/weather archive/tempest.sqlite'` {
		t.Fatalf("registration command = %#v", lines)
	}
}

func TestReadEnvFileRejectsSymlinkOpenPermissionsAndOversize(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not provide the Unix file-mode contract")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.env")
	if err := os.WriteFile(target, []byte("VALUE=ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.env")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readEnvFile(link); err == nil {
		t.Fatal("readEnvFile accepted a symlink")
	}
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readEnvFile(target); err == nil {
		t.Fatal("readEnvFile accepted group-readable permissions")
	}
	oversized := filepath.Join(dir, "oversized.env")
	if err := os.WriteFile(oversized, make([]byte, config.MaxDotenvBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readEnvFile(oversized); err == nil {
		t.Fatal("readEnvFile accepted an oversized file")
	}
}

func TestWritePrivateFileRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink setup requires additional privileges")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.env")
	if err := os.WriteFile(target, []byte("VALUE=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, ".env")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateFile(link, []byte("VALUE=new\n")); err == nil {
		t.Fatal("writePrivateFile replaced a symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "VALUE=old\n" {
		t.Fatalf("target changed to %q", data)
	}
}
