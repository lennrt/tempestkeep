package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/config"
)

func TestCommandsRejectPositionalArgumentsBeforeConfiguration(t *testing.T) {
	// Invalid configuration must not hide the invocation error. A rejected
	// command must not open an archive or enter the interactive setup wizard.
	t.Chdir(t.TempDir())
	t.Setenv("TEMPEST_API_BASE", "invalid-endpoint")
	t.Setenv("TEMPEST_DB", "missing.sqlite")
	for name := range commands {
		for _, suffix := range [][]string{{"unexpected"}, {"unexpected", "--db", "ignored.sqlite"}, {"--", "unexpected"}} {
			t.Run(name+"/"+strings.Join(suffix, "_"), func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				if got := run(append([]string{name}, suffix...), &stdout, &stderr); got != exitUsage {
					t.Fatalf("status=%d, want %d; stderr=%s", got, exitUsage, stderr.String())
				}
				if stdout.Len() != 0 || !strings.Contains(stderr.String(), "positional arguments") {
					t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
			})
		}
	}
}

func TestBuiltinsRejectExtraArguments(t *testing.T) {
	for _, args := range [][]string{{"version", "extra"}, {"--version", "--db", config.DefaultDB}, {"help", "now", "extra"}} {
		var stdout, stderr bytes.Buffer
		if got := run(args, &stdout, &stderr); got != exitUsage || stdout.Len() != 0 {
			t.Fatalf("run(%q): status=%d stdout=%q stderr=%q", args, got, stdout.String(), stderr.String())
		}
	}
}

func TestNowRejectsOverflowingInterval(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("flag.Int rejects this input on 32-bit systems")
	}
	err := cmdNow([]string{"--interval", "9223372037"})
	if !isUsageErr(err) || !strings.Contains(err.Error(), "--interval") {
		t.Fatalf("overflowing interval = %v, want usage error", err)
	}
}

func TestCollectRejectsInvalidDateBeforeDiscovery(t *testing.T) {
	t.Chdir(t.TempDir())
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected discovery", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TEMPEST_TOKEN", "synthetic-token")
	t.Setenv("TEMPEST_API_BASE", srv.URL)
	t.Setenv("TEMPEST_DEVICE_ID", "")
	path := filepath.Join(t.TempDir(), "must-not-exist.sqlite")
	for _, date := range []string{"not-a-date", "1960-01-01", time.Now().AddDate(0, 0, 2).Format(time.DateOnly)} {
		err := cmdCollect([]string{"--db", path, "--backfill-start", date})
		if !isUsageErr(err) || !strings.Contains(err.Error(), "--backfill-start") {
			t.Fatalf("date %q: error=%v, want date usage error", date, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid dates made %d API requests", requests.Load())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid date created an archive or unexpected stat error: %v", err)
	}
}
