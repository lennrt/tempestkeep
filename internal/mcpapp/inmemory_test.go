package mcpapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRunRequiresADataSource(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := Run(ctx, Options{}); err == nil {
		t.Fatal("Run with neither a client nor an archive must fail")
	}
}

// Registration must depend on the resolved client, archive and read-only flag.
// Starting and stopping a session must neither contact the API nor require EOF.
func TestRunOverInMemoryTransport(t *testing.T) {
	t.Setenv("TEMPEST_API_BASE", "invalid-ambient-endpoint")
	t.Setenv("TEMPEST_CACHE_TTL", "invalid-ambient-ttl")
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	liveClient, err := api.New("synthetic-test-token", api.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name                            string
		live, archive, readOnly, legacy bool
	}{
		{name: "archive only", archive: true, readOnly: true},
		{name: "live only", live: true},
		{name: "read-only live archive", live: true, archive: true, readOnly: true},
		{name: "writable live archive", live: true, archive: true},
		{name: "legacy token", live: true, legacy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			opts := Options{ReadOnly: tc.readOnly}
			if tc.live {
				opts.Client = liveClient
				opts.Token = "ignored-when-client-is-supplied"
			}
			if tc.legacy {
				opts.Client = nil
				opts.Token = "synthetic-test-token"
				t.Setenv("TEMPEST_API_BASE", srv.URL)
				t.Setenv("TEMPEST_CACHE_TTL", "0")
			}
			writable := tc.live && tc.archive && !tc.readOnly
			if tc.archive {
				opts.DBPath = filepath.Join(t.TempDir(), "archive.sqlite")
				if !writable {
					writer, err := store.OpenWriter(ctx, opts.DBPath)
					if err != nil {
						t.Fatal(err)
					}
					if err := writer.Close(); err != nil {
						t.Fatal(err)
					}
				}
			}
			serverTransport, clientTransport := mcp.NewInMemoryTransports()
			opts.Transport = serverTransport
			runCtx, stop := context.WithCancel(ctx)
			defer stop()
			done := make(chan struct{})
			var runErr error
			go func() {
				runErr = Run(runCtx, opts)
				close(done)
			}()
			t.Cleanup(func() {
				stop()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("MCP server did not stop during cleanup")
				}
			})
			client := mcp.NewClient(&mcp.Implementation{Name: "tempestkeep-test", Version: "0"}, nil)
			session, err := client.Connect(ctx, clientTransport, nil)
			if err != nil {
				t.Fatalf("connect: %v", err)
			}
			t.Cleanup(func() {
				if err := session.Close(); err != nil {
					t.Errorf("close client session: %v", err)
				}
			})
			listed, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatalf("list tools: %v", err)
			}
			names := make(map[string]bool)
			for _, tool := range listed.Tools {
				names[tool.Name] = true
			}
			for name, want := range map[string]bool{
				"current_conditions": true, "forecast": tc.live,
				"archive_status": tc.archive, "query_sql": tc.archive,
				"sync_archive": writable, "backfill_archive": writable,
			} {
				if names[name] != want {
					t.Errorf("tool %s present=%v, want %v", name, names[name], want)
				}
			}
			stop() // Cancel while the client session is still open, not after EOF.
			select {
			case <-done:
				if !errors.Is(runErr, context.Canceled) {
					t.Fatalf("Run after cancellation = %v", runErr)
				}
			case <-ctx.Done():
				t.Fatal("Run did not stop after cancellation")
			}
			if tc.archive {
				writer, err := store.OpenWriter(ctx, opts.DBPath)
				if err != nil {
					t.Fatalf("reopen archive after shutdown: %v", err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatal("MCP startup or registration contacted the live API")
	}
}

type failingTransport struct{ cause error }

func (t failingTransport) Connect(context.Context) (mcp.Connection, error) { return nil, t.cause }

func TestRunSanitizesTransportErrors(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client, err := api.New("synthetic-test-token")
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("secret-token /private/archive.sqlite https://private.invalid/?device=123")
	err = Run(ctx, Options{Client: client, Transport: failingTransport{cause: cause}})
	if err == nil || errors.Is(err, cause) {
		t.Fatalf("transport error was accepted or retained: %v", err)
	}
	for value := range strings.FieldsSeq(cause.Error()) {
		if strings.Contains(err.Error(), value) {
			t.Fatalf("transport error leaked %q: %v", value, err)
		}
	}
	cancel()
	if err := Run(ctx, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Run = %v", err)
	}
}
