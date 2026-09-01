package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	mcpSubprocessHelperEnv = "TEMPESTKEEP_MCP_SUBPROCESS_HELPER"
	mcpSubprocessDBEnv     = "TEMPESTKEEP_MCP_SUBPROCESS_DB"
)

func TestReadOnlyEnvRejectsInvalidValue(t *testing.T) {
	t.Setenv("TEMPEST_READ_ONLY", "true")
	if got, err := readOnlyEnv(); err != nil || !got {
		t.Fatalf("readOnlyEnv(true) = %v, %v", got, err)
	}
	t.Setenv("TEMPEST_READ_ONLY", "off")
	if got, err := readOnlyEnv(); err != nil || got {
		t.Fatalf("readOnlyEnv(off) = %v, %v", got, err)
	}
	t.Setenv("TEMPEST_READ_ONLY", "tru")
	if _, err := readOnlyEnv(); err == nil {
		t.Fatal("readOnlyEnv accepted a misspelled security setting")
	}
}

func TestMCPHelpStopsBeforeConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := cmdMCP([]string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("cmdMCP(-h) = %v, want flag.ErrHelp", err)
	}
}

func TestMCPRejectsPositionalArguments(t *testing.T) {
	err := cmdMCP([]string{"unexpected"})
	if err == nil || !isUsageErr(err) {
		t.Fatalf("cmdMCP(positional) = %v, want usage error", err)
	}
}

// TestE2ECommandMCPStdio crosses the command-dispatch boundary over real
// subprocess pipes. Successful MCP negotiation proves that startup diagnostics
// did not pollute stdout with non-protocol data.
func TestE2ECommandMCPStdio(t *testing.T) {
	t.Run("protocol and clean EOF", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()

		cmd, stderr, dbPath := newMCPSubprocess(t, ctx)
		client := mcp.NewClient(&mcp.Implementation{Name: "command-e2e", Version: "test"}, nil)
		session, err := client.Connect(ctx, &mcp.CommandTransport{
			Command:           cmd,
			TerminateDuration: 2 * time.Second,
		}, nil)
		if err != nil {
			t.Fatalf("connect to tempestkeep mcp: %v\nstderr:\n%s", err, stderr)
		}

		init := session.InitializeResult()
		if init.ServerInfo == nil || init.ServerInfo.Name != "tempestkeep" {
			t.Fatalf("server info = %+v, want name tempestkeep", init.ServerInfo)
		}
		if init.ProtocolVersion == "" {
			t.Fatal("server negotiated an empty MCP protocol version")
		}

		result, err := session.ListTools(ctx, nil)
		if err != nil {
			t.Fatalf("list tools: %v\nstderr:\n%s", err, stderr)
		}
		tools := make(map[string]bool, len(result.Tools))
		for _, tool := range result.Tools {
			tools[tool.Name] = true
		}
		for _, name := range []string{"archive_status", "current_conditions", "query_sql"} {
			if !tools[name] {
				t.Errorf("archive tool %q is not registered", name)
			}
		}
		for _, name := range []string{"backfill_archive", "forecast", "sync_archive"} {
			if tools[name] {
				t.Errorf("tool %q is registered without a token", name)
			}
		}

		// Closing the client closes the child's stdin. The MCP server must treat
		// that EOF as a normal shutdown and exit before the transport deadline.
		if err := session.Close(); err != nil {
			t.Fatalf("close MCP session: %v\nstderr:\n%s", err, stderr)
		}
		if cmd.ProcessState == nil || !cmd.ProcessState.Success() {
			t.Fatalf("MCP subprocess state = %v, want successful exit", cmd.ProcessState)
		}
		assertMCPDiagnostics(t, stderr.String(), dbPath)
	})

	t.Run("interrupt cancellation", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("os.Interrupt is not implemented on Windows")
		}

		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()

		cmd, stderr, dbPath := newMCPSubprocess(t, ctx)
		client := mcp.NewClient(&mcp.Implementation{Name: "command-e2e", Version: "test"}, nil)
		session, err := client.Connect(ctx, &mcp.CommandTransport{
			Command:           cmd,
			TerminateDuration: 2 * time.Second,
		}, nil)
		if err != nil {
			t.Fatalf("connect to tempestkeep mcp: %v\nstderr:\n%s", err, stderr)
		}

		wait := make(chan error, 1)
		go func() { wait <- session.Wait() }()
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			t.Fatalf("interrupt MCP subprocess: %v", err)
		}
		select {
		case <-wait:
		case <-ctx.Done():
			t.Fatalf("MCP subprocess did not stop after interrupt: %v", ctx.Err())
		}

		// The real command reports cancellation as a non-zero interrupted run.
		// Close reaps the child; its error is the expected exit status.
		_ = session.Close()
		if cmd.ProcessState == nil {
			t.Fatal("MCP subprocess was not reaped after interrupt")
		}
		if got := stderr.String(); !strings.Contains(got, "tempestkeep: context canceled") {
			t.Fatalf("cancellation diagnostic is missing\nstderr:\n%s", got)
		}
		if strings.Contains(stderr.String(), dbPath) {
			t.Fatalf("cancellation diagnostic exposed the archive path\nstderr:\n%s", stderr)
		}
	})
}

// TestMCPSubprocessHelper turns this test binary into the real tempestkeep
// command. os.Exit suppresses the testing package's PASS line on protocol
// stdout after a normal EOF shutdown.
func TestMCPSubprocessHelper(t *testing.T) {
	if os.Getenv(mcpSubprocessHelperEnv) != "1" {
		return
	}
	dbPath := os.Getenv(mcpSubprocessDBEnv)
	if dbPath == "" {
		fmt.Fprintln(os.Stderr, "MCP subprocess helper has no archive")
		os.Exit(2)
	}
	os.Args = []string{"tempestkeep", "mcp", "--db", dbPath, "--read-only"}
	main()
	os.Exit(0)
}

func newMCPSubprocess(t *testing.T, ctx context.Context) (*exec.Cmd, *bytes.Buffer, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "archive.sqlite")
	writer, err := store.OpenWriter(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("create MCP test archive: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close MCP test archive: %v", err)
	}

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMCPSubprocessHelper$")
	cmd.Dir = dir
	cmd.Env = mcpSubprocessEnv(dbPath)
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	t.Cleanup(func() {
		if cmd.Process == nil || cmd.ProcessState != nil {
			return
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd, stderr, dbPath
}

func mcpSubprocessEnv(dbPath string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok && (strings.HasPrefix(key, "TEMPEST_") || key == mcpSubprocessHelperEnv || key == mcpSubprocessDBEnv) {
			continue
		}
		env = append(env, entry)
	}
	return append(env,
		mcpSubprocessHelperEnv+"=1",
		mcpSubprocessDBEnv+"="+dbPath,
	)
}

func assertMCPDiagnostics(t *testing.T, diagnostics, dbPath string) {
	t.Helper()
	for _, want := range []string{"archive read access is ready (read-only)", "tempestkeep mcp"} {
		if !strings.Contains(diagnostics, want) {
			t.Errorf("MCP stderr is missing %q\nstderr:\n%s", want, diagnostics)
		}
	}
	if dbPath != "" && strings.Contains(diagnostics, dbPath) {
		t.Errorf("MCP diagnostics exposed the archive path\nstderr:\n%s", diagnostics)
	}
}
