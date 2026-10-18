package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/internal/mcpapp"
	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPInvalidDeviceFlagsFailBeforeConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	// A directory cannot be loaded as dotenv. Invalid flags must still report
	// usage errors rather than this configuration failure.
	if err := os.Mkdir(".env", 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEMPEST_DEVICE_ID", "777")
	path := filepath.Join(t.TempDir(), "must-not-exist.sqlite")
	for _, value := range []string{"0", "-1", "", "invalid", "9999999999999999999999999"} {
		t.Run(value, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if status := run([]string{"mcp", "--db", path, "--device-id=" + value}, &stdout, &stderr); status != exitUsage {
				t.Fatalf("invalid flag status=%d stderr=%s", status, stderr.String())
			}
			if strings.Contains(stderr.String(), "dotenv") || stdout.Len() != 0 {
				t.Fatalf("invalid flag read configuration: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid flag initialized archive: %v", err)
	}
}

func TestMCPInvalidDeviceEnvironmentFailsBeforeArchiveAndAPIConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("TEMPEST_TOKEN", "synthetic-token")
	t.Setenv("TEMPEST_API_BASE", "invalid-api-endpoint")
	path := filepath.Join(t.TempDir(), "must-not-exist.sqlite")
	for _, value := range []string{"-1", "invalid", "9999999999999999999999999"} {
		t.Setenv("TEMPEST_DEVICE_ID", value)
		err := cmdMCP([]string{"--db", path})
		if err == nil || !strings.Contains(err.Error(), "TEMPEST_DEVICE_ID") {
			t.Fatalf("invalid device environment = %v", err)
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid environment initialized archive: %v", err)
	}
}

const deviceCommandHelper = "TEMPESTKEEP_DEVICE_COMMAND_HELPER"
const deviceCommandFlag = "TEMPESTKEEP_DEVICE_COMMAND_FLAG"

func TestMCPDeviceCommandHelper(t *testing.T) {
	if os.Getenv(deviceCommandHelper) != "1" {
		return
	}
	os.Args = []string{"tempestkeep", "mcp"}
	if value := os.Getenv(deviceCommandFlag); value != "" {
		os.Args = append(os.Args, "--device-id="+value)
	}
	main()
}

func TestE2EMCPDeviceConfigurationPrecedence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/stations" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"stations": []api.Station{
			{StationID: 123, Name: "First station", Devices: []api.Device{{DeviceID: 456, DeviceType: "ST"}}},
			{StationID: 789, Name: "Selected station", Devices: []api.Device{{DeviceID: 777, DeviceType: "ST"}}},
		}})
	}))
	t.Cleanup(server.Close)
	for _, tc := range []struct {
		name, flag, env, dotenv string
		want                    int
	}{
		{name: "discovery", want: 123},
		{name: "zero environment", env: "0", want: 123},
		{name: "environment selection", env: "777", want: 789},
		{name: "flag overrides environment", flag: "456", env: "777", want: 123},
		{name: "flag overrides malformed environment", flag: "777", env: "invalid", want: 789},
		{name: "dotenv selection", dotenv: "777", want: 789},
		{name: "environment overrides dotenv", env: "456", dotenv: "777", want: 123},
		{name: "zero environment overrides dotenv", env: "0", dotenv: "777", want: 123},
		{name: "flag overrides dotenv", flag: "777", dotenv: "invalid", want: 789},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMCPDeviceCommandHelper$")
			cmd.Dir = t.TempDir()
			if tc.dotenv != "" {
				if err := os.WriteFile(filepath.Join(cmd.Dir, ".env"), []byte("TEMPEST_DEVICE_ID="+tc.dotenv+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for _, entry := range os.Environ() {
				key, _, _ := strings.Cut(entry, "=")
				if strings.HasPrefix(key, "TEMPEST_") || key == deviceCommandHelper || key == deviceCommandFlag {
					continue
				}
				cmd.Env = append(cmd.Env, entry)
			}
			cmd.Env = append(cmd.Env, deviceCommandHelper+"=1", deviceCommandFlag+"="+tc.flag,
				"TEMPEST_TOKEN=synthetic-command-token", "TEMPEST_API_BASE="+server.URL, "TEMPEST_CACHE_TTL=0")
			if tc.env != "" {
				cmd.Env = append(cmd.Env, "TEMPEST_DEVICE_ID="+tc.env)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			cs, err := mcp.NewClient(&mcp.Implementation{Name: "selection-command", Version: "test"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: 2 * time.Second}, nil)
			if err != nil {
				t.Fatalf("connect selection command: %v; stderr=%s", err, &stderr)
			}
			defer func() {
				if err := cs.Close(); err != nil {
					t.Errorf("close selection command: %v", err)
				}
			}()
			result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "station_details"})
			if err != nil || result.IsError {
				t.Fatalf("selected station call: error=%v result=%+v", err, result)
			}
			data, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			var details mcpapp.StationDetailsOut
			if err := json.Unmarshal(data, &details); err != nil {
				t.Fatal(err)
			}
			if details.StationID != tc.want {
				t.Fatalf("configuration selected station %d, want %d", details.StationID, tc.want)
			}
		})
	}
}
