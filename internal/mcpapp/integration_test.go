package mcpapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "modernc.org/sqlite"
)

// wantProtocol is the MCP revision we expect the go-sdk to negotiate. It pins
// our compliance target; a change here should be a deliberate SDK upgrade, not
// a surprise. (Current spec as of 2026-07; see the package doc comment.)
const wantProtocol = "2026-07-28"

// makeTestArchive writes a minimal obs_st archive (the columns the store reads)
// with two rows on different local days, and returns its path. This stands in
// for what the collector produces, so the integration test needs no real data.
func makeTestArchive(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tempest.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open temp db: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close temp db: %v", err)
		}
	}()
	ctx := t.Context()

	if _, err := db.ExecContext(ctx, `CREATE TABLE obs_st (
		epoch INTEGER NOT NULL, wind_lull REAL, wind_avg REAL, wind_gust REAL,
		wind_dir REAL, pressure_mb REAL, air_temp_c REAL, humidity REAL,
		illuminance_lux REAL, uv REAL, solar_wm2 REAL, rain_mm REAL,
		strike_dist_km REAL, strike_count REAL, battery_v REAL
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	rows := []struct {
		epoch             int64
		tempC, gust, rain float64
	}{
		{1700000000, 20, 5, 1.0}, // 20°C -> 68°F, gust 5 m/s
		{1700086400, 25, 8, 0.0}, // 25°C -> 77°F, gust 8 m/s (next day)
	}
	for _, r := range rows {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO obs_st (epoch, air_temp_c, humidity, wind_gust, rain_mm) VALUES (?,?,?,?,?)`,
			r.epoch, r.tempC, 50.0, r.gust, r.rain); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	return path
}

// connectArchiveServer opens the given archive read-only, registers the
// archive-backed tools on a fresh server (no token), and returns an initialized
// client session wired to it over an in-memory transport.
func connectArchiveServer(t *testing.T, ctx context.Context, dbPath string) *mcp.ClientSession {
	t.Helper()
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	closeOnCleanup(t, st)

	srv := mcp.NewServer(&mcp.Implementation{Name: "tempestkeep", Version: "test"}, nil)
	registerTools(srv, nil, st) // live=nil -> archive-only

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

// TestIntegrationArchiveTools drives the server end to end over the real MCP
// protocol: it negotiates, lists tools, and calls them, asserting on the
// structured output, the same path a real agent (Claude Desktop/Code) takes.
func TestIntegrationArchiveTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cs := connectArchiveServer(t, ctx, makeTestArchive(t))

	// The SDK should negotiate our pinned protocol revision.
	if got := cs.InitializeResult().ProtocolVersion; got != wantProtocol {
		t.Errorf("negotiated protocol = %q, want %q", got, wantProtocol)
	}

	// tools/list: archive tools present with output schemas; live tools absent.
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	got := map[string]*mcp.Tool{}
	for _, tl := range tools.Tools {
		got[tl.Name] = tl
	}
	for _, name := range []string{"current_conditions", "station_info", "daily_summary", "records"} {
		tl, ok := got[name]
		if !ok {
			t.Errorf("archive tool %q not registered", name)
			continue
		}
		if tl.OutputSchema == nil {
			t.Errorf("tool %q has no output schema", name)
		}
	}
	for _, name := range []string{"list_stations", "forecast", "station_details"} {
		if _, ok := got[name]; ok {
			t.Errorf("live tool %q should be absent without a token", name)
		}
	}

	// records: structured content, with SI->US conversion applied.
	t.Run("records", func(t *testing.T) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "records", Arguments: map[string]any{}})
		if err != nil {
			t.Fatalf("call records: %v", err)
		}
		if res.IsError {
			t.Fatalf("records returned isError; content=%v", res.Content)
		}
		var rec store.Records
		decodeStructured(t, res, &rec)
		if rec.HottestF == nil || !almost(*rec.HottestF, 77) {
			t.Errorf("hottest_f = %v, want 77 (25°C)", rec.HottestF)
		}
		if rec.ColdestF == nil || !almost(*rec.ColdestF, 68) {
			t.Errorf("coldest_f = %v, want 68 (20°C)", rec.ColdestF)
		}
	})

	// current_conditions: falls back to the newest archive row (no token).
	t.Run("current_conditions", func(t *testing.T) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "current_conditions", Arguments: map[string]any{}})
		if err != nil {
			t.Fatalf("call current_conditions: %v", err)
		}
		var cond ConditionsOut
		decodeStructured(t, res, &cond)
		if cond.Source != "archive" {
			t.Errorf("source = %q, want archive", cond.Source)
		}
		if cond.TempF == nil || !almost(*cond.TempF, 77) { // newest row is 25°C
			t.Errorf("temp_f = %v, want 77", cond.TempF)
		}
	})
}

// decodeStructured re-marshals a tool result's StructuredContent into v, so we
// can assert on it with the same types the server returned.
func decodeStructured(t *testing.T, res *mcp.CallToolResult, v any) {
	t.Helper()
	if res.StructuredContent == nil {
		t.Fatal("result has no structured content")
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
}
