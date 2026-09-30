package mcpapp

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCrossSourceEnrichmentRequiresMatchingDevice(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		deviceID              int
		unknown, newerNoValue bool
		pressureOffset        int64
		wantName, wantPT      bool
	}{
		{name: "matched", deviceID: 456, wantName: true, wantPT: true},
		{name: "mismatched", deviceID: 999},
		{name: "unknown", deviceID: 456, unknown: true},
		{name: "stale pressure", deviceID: 456, pressureOffset: -30 * 86400, wantName: true},
		{name: "stale pressure with fresh temperature", deviceID: 456, pressureOffset: -30 * 86400, newerNoValue: true, wantName: true},
		{name: "future pressure", deviceID: 456, pressureOffset: 3600, wantName: true},
		{name: "recent pressure", deviceID: 456, pressureOffset: -3599, wantName: true, wantPT: true},
		{name: "pressure at stale boundary", deviceID: 456, pressureOffset: -3600, wantName: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const liveEpoch = int64(1700000000)
			lastEpoch := liveEpoch + tc.pressureOffset
			path := filepath.Join(t.TempDir(), "archive.sqlite")
			writer, err := store.OpenWriter(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := writer.InsertObs(t.Context(), tc.deviceID, []model.DeviceObs{
				{Epoch: lastEpoch - 3*3600, PressureMb: new(1018.0)},
				{Epoch: lastEpoch, PressureMb: new(1012.0)},
			}); err != nil {
				t.Fatal(err)
			}
			wantRows := int64(2)
			if tc.newerNoValue {
				if _, err := writer.InsertObs(t.Context(), tc.deviceID, []model.DeviceObs{{Epoch: liveEpoch, AirTempC: new(20.0)}}); err != nil {
					t.Fatal(err)
				}
				wantRows++
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if tc.unknown {
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(t.Context(), `ALTER TABLE obs_st RENAME COLUMN device_id TO legacy_device`); err != nil {
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}
			st, err := store.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			closeOnCleanup(t, st)
			mock := liveMockServer(t)
			t.Cleanup(mock.Close)
			client, err := api.New("synthetic-token", api.WithBaseURL(mock.URL))
			if err != nil {
				t.Fatal(err)
			}
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
			registerTools(server, &liveSource{client: client}, st)
			clientTransport, serverTransport := mcp.NewInMemoryTransports()
			serverSession, err := server.Connect(t.Context(), serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			closeOnCleanup(t, serverSession)
			cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(t.Context(), clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			closeOnCleanup(t, cs)
			var info StationInfoOut
			callTool(t, t.Context(), cs, "station_info", nil, &info)
			if (info.Name != "") != tc.wantName || (info.StationID != 0) != tc.wantName || info.Observations != wantRows {
				t.Errorf("archive identity was not matched: %+v", info)
			}
			var conditions ConditionsOut
			callTool(t, t.Context(), cs, "current_conditions", nil, &conditions)
			if conditions.Source != "live" || conditions.Station == "" || conditions.TempF == nil {
				t.Errorf("independent live observations became unavailable: %+v", conditions)
			}
			if (conditions.PressureTrend3hInHg != nil) != tc.wantPT || (conditions.PressureTrend != "") != tc.wantPT {
				t.Errorf("archive trend was not matched to live observation: %+v", conditions)
			}
		})
	}
}

func TestCurrentConditionsArchivePressureFreshnessAndRate(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		pressureSpan, latestDelay int64
		wantRate                  bool
	}{
		{name: "too little history", pressureSpan: 3*3600 - 1},
		{name: "exact three hours", pressureSpan: 3 * 3600, wantRate: true},
		{name: "sparse six hours", pressureSpan: 6 * 3600, wantRate: true},
		{name: "recent pressure", pressureSpan: 3 * 3600, latestDelay: 3599, wantRate: true},
		{name: "stale boundary", pressureSpan: 3 * 3600, latestDelay: 3600},
		{name: "new temperature stale pressure", pressureSpan: 3 * 3600, latestDelay: 30 * 86400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const pressureAt = int64(1700000000)
			path := filepath.Join(t.TempDir(), "archive.sqlite")
			writer, err := store.OpenWriter(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			rows := []model.DeviceObs{
				{Epoch: pressureAt - tc.pressureSpan, PressureMb: new(1018.0)},
				{Epoch: pressureAt, PressureMb: new(1012.0), AirTempC: new(20.0)},
			}
			if tc.latestDelay > 0 {
				rows = append(rows, model.DeviceObs{Epoch: pressureAt + tc.latestDelay, AirTempC: new(21.0)})
			}
			if _, err := writer.InsertObs(t.Context(), 456, rows); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			cs := connectArchiveServer(t, t.Context(), path)
			var out ConditionsOut
			callTool(t, t.Context(), cs, "current_conditions", nil, &out)
			if out.Source != "archive" || out.TempF == nil {
				t.Fatalf("archive source unavailable: %+v", out)
			}
			if (out.PressureTrend3hInHg != nil) != tc.wantRate || (out.PressureTrend != "") != tc.wantRate {
				t.Fatalf("pressure freshness did not match displayed observation: %+v", out)
			}
			if tc.wantRate {
				want := model.MbToInHg(-6 * (3 * 3600) / float64(tc.pressureSpan))
				if math.Abs(*out.PressureTrend3hInHg-want) > 1e-9 {
					t.Fatalf("three-hour pressure rate=%v want%v for span%dsec", *out.PressureTrend3hInHg, want, tc.pressureSpan)
				}
			}
			// The dedicated historical tool retains the actual span and delta,
			// even when this trend cannot enrich the latest conditions response.
			if tc.pressureSpan >= 3*3600 {
				var history PressureTrendOut
				callTool(t, t.Context(), cs, "pressure_trend", nil, &history)
				if history.At != pressureAt || math.Abs(history.ChangeInHg-model.MbToInHg(-6)) > 1e-9 {
					t.Fatalf("historical pressure trend changed: %+v", history)
				}
			}
		})
	}
}
