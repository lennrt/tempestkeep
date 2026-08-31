package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestLoadDotenvDoesNotOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("TEMPEST_CFG_A=fromfile\n# comment\n\nTEMPEST_CFG_B=preset\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("TEMPEST_CFG_A"); err != nil {
		t.Fatalf("unset test variable: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Unsetenv("TEMPEST_CFG_A"); err != nil {
			t.Errorf("unset test variable: %v", err)
		}
	})
	t.Setenv("TEMPEST_CFG_B", "already-set")

	if err := LoadDotenv(t.Context(), path); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("TEMPEST_CFG_A"); got != "fromfile" {
		t.Errorf("A = %q, want fromfile", got)
	}
	if got := os.Getenv("TEMPEST_CFG_B"); got != "already-set" {
		t.Errorf("B = %q, want already-set (env must win over file)", got)
	}
}

func TestLoadDotenvRejectsSymlinkAndOpenPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not provide the Unix file-mode contract")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.env")
	if err := os.WriteFile(target, []byte("TEMPEST_CFG_PRIVATE=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.env")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := LoadDotenv(t.Context(), link); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("symlink error = %v, want ErrInvalidConfig", err)
	}
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := LoadDotenv(t.Context(), target); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("permission error = %v, want ErrInvalidConfig", err)
	}
}

func TestLoadDotenvIOErrorDoesNotRetainPath(t *testing.T) {
	marker := "private-path-marker"
	path := filepath.Join(t.TempDir(), strings.Repeat(marker, 32))
	err := LoadDotenv(t.Context(), path)
	if !errors.Is(err, ErrConfigIO) {
		t.Fatalf("error = %v, want ErrConfigIO", err)
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("error retains path marker: %v", err)
	}
}

func TestParseDotenvRejectsLimits(t *testing.T) {
	oversizedLine := []byte("A=" + strings.Repeat("x", maxDotenvLineBytes) + "\n")
	if _, err := ParseDotenv(oversizedLine); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("oversized line error = %v, want ErrInvalidConfig", err)
	}
	var variables strings.Builder
	for index := range maxDotenvVariables + 1 {
		variables.WriteString("KEY_")
		variables.WriteString(strconv.Itoa(index))
		variables.WriteString("=value\n")
	}
	if _, err := ParseDotenv([]byte(variables.String())); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("variable count error = %v, want ErrInvalidConfig", err)
	}
}

func TestParseDotenvQuotesExportAndInvalidKeys(t *testing.T) {
	got, err := ParseDotenv([]byte("export GOOD=plain\nPATH_VALUE=\"a path/#1\"\nSINGLE='kept value'\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"GOOD":       "plain",
		"PATH_VALUE": "a path/#1",
		"SINGLE":     "kept value",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseDotenv() = %#v, want %#v", got, want)
	}
	if _, err := ParseDotenv([]byte("BAD-KEY=nope\n")); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid key error = %v, want ErrInvalidConfig", err)
	}
}

func TestFormatDotenvValueRoundTrip(t *testing.T) {
	for _, value := range []string{"", "plain", "a path", "\v", "\u00a0wrapped\u00a0", `C:\Users\weather`, "hash#quote\"", "line1\nline2"} {
		formatted, err := FormatDotenvValue(value)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := ParseDotenv([]byte("VALUE=" + formatted + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		if got := parsed["VALUE"]; got != value {
			t.Errorf("round trip %q = %q", value, got)
		}
	}
}

func FuzzDotenvValueRoundTrip(f *testing.F) {
	for _, value := range []string{"", "plain", " spaces # and quotes ", "line1\nline2", `C:\Users\weather`} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		formatted, err := FormatDotenvValue(value)
		if err != nil {
			if strings.IndexByte(value, 0) >= 0 || len(value) > maxDotenvLineBytes {
				return
			}
			t.Fatal(err)
		}
		parsed, err := ParseDotenv([]byte("VALUE=" + formatted + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		if got := parsed["VALUE"]; got != value {
			t.Fatalf("round trip %q = %q", value, got)
		}
	})
}

func TestFirstNonEmpty(t *testing.T) {
	if got := FirstNonEmpty("", "", "x", "y"); got != "x" {
		t.Errorf("got %q, want x", got)
	}
	if got := FirstNonEmpty("", ""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveDB(t *testing.T) {
	t.Setenv("TEMPEST_DB", "")
	if got, err := ResolveDB(t.Context(), "/explicit/path.sqlite"); err != nil || got != "/explicit/path.sqlite" {
		t.Errorf("explicit flag ignored: %q", got)
	}
	t.Setenv("TEMPEST_DB", "/from/env.sqlite")
	if got, err := ResolveDB(t.Context(), ""); err != nil || got != "/from/env.sqlite" {
		t.Errorf("env not used: %q", got)
	}

	// With neither flag nor env, ResolveDB returns the default only if it exists.
	t.Setenv("TEMPEST_DB", "")
	dir := t.TempDir()
	t.Chdir(dir)
	if got, err := ResolveDB(t.Context(), ""); err != nil || got != "" {
		t.Errorf("with no default file present, got %q, want empty", got)
	}
	if err := os.WriteFile(filepath.Join(dir, DefaultDB), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveDB(t.Context(), ""); err != nil || got != DefaultDB {
		t.Errorf("with default file present, got %q, want %q", got, DefaultDB)
	}
}
