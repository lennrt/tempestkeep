package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestRunExitStatuses pins the exit-status contract documented in the README:
// 0 for success and help, 2 for usage errors, and stdout/stderr routing.
func TestRunExitStatuses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		wantStatus int
		wantOut    string
		wantErr    string
	}{
		{name: "version", args: []string{"version"}, wantStatus: exitOK, wantOut: "tempestkeep "},
		{name: "version flag", args: []string{"--version"}, wantStatus: exitOK, wantOut: "tempestkeep "},
		{name: "help", args: []string{"help"}, wantStatus: exitOK, wantOut: "Usage:"},
		{name: "help flag", args: []string{"-h"}, wantStatus: exitOK, wantOut: "Usage:"},
		{name: "no arguments", args: nil, wantStatus: exitUsage, wantErr: "Usage:"},
		{name: "unknown command", args: []string{"frobnicate"}, wantStatus: exitUsage, wantErr: `unknown command "frobnicate"`},
		{name: "near miss suggests", args: []string{"colect"}, wantStatus: exitUsage, wantErr: `did you mean "collect"`},
		{name: "help for unknown", args: []string{"help", "colect"}, wantStatus: exitUsage, wantErr: `did you mean "collect"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tc.args, &stdout, &stderr); got != tc.wantStatus {
				t.Fatalf("run(%q) = %d, want %d\nstdout: %s\nstderr: %s", tc.args, got, tc.wantStatus, stdout.String(), stderr.String())
			}
			if tc.wantOut != "" && !strings.Contains(stdout.String(), tc.wantOut) {
				t.Fatalf("stdout %q does not contain %q", stdout.String(), tc.wantOut)
			}
			if tc.wantErr != "" && !strings.Contains(stderr.String(), tc.wantErr) {
				t.Fatalf("stderr %q does not contain %q", stderr.String(), tc.wantErr)
			}
			if tc.wantOut != "" && stderr.Len() != 0 {
				t.Fatalf("successful invocation wrote to stderr: %q", stderr.String())
			}
			if tc.wantErr != "" && stdout.Len() != 0 {
				t.Fatalf("failed invocation wrote to stdout: %q", stdout.String())
			}
		})
	}
}

// TestRunUsageErrorStatus checks that a usageErr from a command maps to status 2.
func TestRunUsageErrorStatus(t *testing.T) {
	var stderr bytes.Buffer
	if got := run([]string{"mcp", "extra-positional"}, &bytes.Buffer{}, &stderr); got != exitUsage {
		t.Fatalf("positional argument to mcp: status %d, want %d (stderr: %s)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "positional") {
		t.Fatalf("stderr %q does not explain the usage error", stderr.String())
	}
}

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestRunRuntimeErrorStatus(t *testing.T) {
	var stderr bytes.Buffer
	if got := run([]string{"version"}, failedOutput{}, &stderr); got != exitFailure {
		t.Fatalf("failed output: status %d, want %d", got, exitFailure)
	}
	if !strings.Contains(stderr.String(), io.ErrClosedPipe.Error()) {
		t.Fatalf("missing runtime failure diagnostic: %q", stderr.String())
	}
}
