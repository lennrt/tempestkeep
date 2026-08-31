package main

// End-to-end test of the archive write path, exercised the way a real agent
// drives it (over the MCP protocol) but with the WeatherFlow REST API replaced
// by an in-process mock. The full stack runs for real:
//
//	MCP client → in-memory transport → server tool → collect.Backfiller →
//	api.Client → HTTP (httptest) → store.Writer → SQLite (temp file) → back out
//
// Nothing is stubbed except the network boundary and the clock-bound history the
// mock chooses to expose, so this catches wiring regressions unit tests can't.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const e2eStationsJSON = `{"stations":[{
	"station_id":123,"name":"E2E Station","timezone":"UTC",
	"devices":[{"device_id":456,"device_type":"ST","serial_number":"ST-E2E"}]
}]}`

// mockHistory serves a finite history: one observation on the hour within
// [start, end]. Requests for windows outside that range come back empty, which is
// exactly what a backward walk needs to discover that history has ended.
type mockHistory struct {
	start, end int64
	hits       int
}

func (m *mockHistory) obsIn(reqStart, reqEnd int64) []int64 {
	lo, hi := max64(reqStart, m.start), min64(reqEnd, m.end)
	var out []int64
	for e := ceilHour(lo); e <= hi; e += 3600 {
		out = append(out, e)
	}
	return out
}

func (m *mockHistory) total() int { return len(m.obsIn(m.start, m.end)) }

func (m *mockHistory) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/stations":
			_, _ = w.Write([]byte(e2eStationsJSON))
		case pathHasPrefix(r.URL.Path, "/observations/device/"):
			m.hits++
			start, _ := strconv.ParseInt(r.URL.Query().Get("time_start"), 10, 64)
			end, _ := strconv.ParseInt(r.URL.Query().Get("time_end"), 10, 64)
			writeObs(w, m.obsIn(start, end))
		default:
			http.NotFound(w, r)
		}
	}))
}

// writeObs emits a WeatherFlow-shaped obs_st payload: each row is the 18-element
// array with epoch (0), wind_gust (3), and air_temp_c (7) populated, rest null.
func writeObs(w http.ResponseWriter, epochs []int64) {
	obs := make([][]any, 0, len(epochs))
	for i, e := range epochs {
		row := make([]any, 18)
		row[0] = e
		row[3] = 3.0 + float64(i%5)   // wind_gust, varied so records/gaps have signal
		row[7] = 15.0 + float64(i%10) // air_temp_c
		obs = append(obs, row)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"obs": obs})
}

// connectWritableServer builds the server exactly as main() does in write mode
// (live + read-only store + writer) and returns an initialized in-memory client.
func connectWritableServer(t *testing.T, ctx context.Context, token, dbPath string) *mcp.ClientSession {
	t.Helper()
	apiClient, err := newAPIClient(token)
	if err != nil {
		t.Fatal(err)
	}
	live := &liveSource{client: apiClient}
	writer, err := store.OpenWriter(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	closeOnCleanup(t, writer)
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	closeOnCleanup(t, st)

	srv := mcp.NewServer(&mcp.Implementation{Name: "tempest-mcp", Version: "test"}, nil)
	registerTools(srv, live, st)
	registerArchiveTools(srv, live, writer)

	clientT, serverT := mcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	closeOnCleanup(t, serverSession)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	closeOnCleanup(t, cs)
	return cs
}

func TestE2EArchiveWriteTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().Unix()
	hist := &mockHistory{start: now - 20*86400, end: now} // 20 days of hourly data ending ~now
	srv := hist.server(t)
	t.Cleanup(srv.Close)

	// Point the real api.Client at the mock and make backfills run instantly.
	t.Setenv("TEMPEST_API_BASE", srv.URL)
	t.Setenv("TEMPEST_THROTTLE_MS", "0")
	t.Setenv("TEMPEST_CACHE_TTL", "0")

	dbPath := filepath.Join(t.TempDir(), "e2e.sqlite")
	cs := connectWritableServer(t, ctx, "e2e-token", dbPath)

	// The write tools must be advertised in write mode.
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tl := range tools.Tools {
		byName[tl.Name] = tl
	}
	for _, want := range []string{"backfill_archive", "sync_archive", "archive_status"} {
		if byName[want] == nil {
			t.Errorf("write tool %q not registered", want)
		}
	}
	// The write tools must advertise the append-only safety model: idempotent
	// and never destructive.
	for _, name := range []string{"backfill_archive", "sync_archive"} {
		tl := byName[name]
		if tl == nil {
			continue
		}
		a := tl.Annotations
		if a == nil || !a.IdempotentHint || a.DestructiveHint == nil || *a.DestructiveHint {
			t.Errorf("tool %q must be annotated idempotent and non-destructive, got %+v", name, a)
		}
	}

	// 1) A fresh archive reports nothing stored.
	var status ArchiveStatusOut
	callTool(t, ctx, cs, "archive_status", nil, &status)
	if status.Observations != 0 {
		t.Fatalf("fresh archive observations = %d, want 0", status.Observations)
	}

	// 2) Walk history backward, no arguments, until has_more is false, per the
	//    documented agent contract. It must terminate and store every hour.
	calls := 0
	for {
		var out BackfillOut
		callTool(t, ctx, cs, "backfill_archive", map[string]any{}, &out)
		calls++
		if !out.HasMore {
			break
		}
		if calls > 12 {
			t.Fatal("backfill_archive never reported has_more=false (did the walk-back fail to terminate?)")
		}
	}
	wantRows := int64(hist.total())
	callTool(t, ctx, cs, "archive_status", nil, &status)
	if status.Observations != wantRows {
		t.Errorf("after backfill observations = %d, want %d", status.Observations, wantRows)
	}
	if !status.BackfillComplete {
		t.Error("backfill_complete = false, want true after the walk exhausted history")
	}
	if len(status.Gaps) != 0 {
		t.Errorf("gaps = %+v, want none (hourly data has no >1h holes)", status.Gaps)
	}
	if !status.Fresh {
		t.Error("fresh = false, want true (newest obs is ~now)")
	}

	// 3) Idempotency: a second full walk from scratch adds nothing new. (The cursor
	//    is complete, so this returns immediately.)
	var again BackfillOut
	callTool(t, ctx, cs, "backfill_archive", map[string]any{}, &again)
	if again.RowsAdded != 0 {
		t.Errorf("re-backfill added %d rows, want 0 (idempotent)", again.RowsAdded)
	}

	// 4) sync_archive is safe to call and reports the newest stored observation.
	var sync SyncOut
	callTool(t, ctx, cs, "sync_archive", nil, &sync)
	if sync.LastObs == "" {
		t.Error("sync_archive returned no last_obs")
	}

	// 5) The read-only history tools see the same data over the same session.
	var rec store.Records
	callTool(t, ctx, cs, "records", nil, &rec)
	if rec.HottestF == nil {
		t.Error("records.hottest_f is nil, want a value from the backfilled data")
	}
}

// callTool invokes a tool, fails on transport or tool error, and decodes the
// structured result into out (pass nil to ignore it).
func callTool(t *testing.T, ctx context.Context, cs *mcp.ClientSession, name string, args map[string]any, out any) {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s returned isError; content=%v", name, res.Content)
	}
	if out != nil {
		decodeStructured(t, res, out)
	}
}

// ---- small helpers ----------------------------------------------------------

func ceilHour(x int64) int64 { return ((x + 3599) / 3600) * 3600 }

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func pathHasPrefix(path, prefix string) bool {
	return strings.HasPrefix(path, prefix)
}
