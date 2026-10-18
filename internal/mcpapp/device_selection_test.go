package mcpapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func selectionStations() []api.Station {
	return []api.Station{
		{StationID: 123, Name: "First station", Devices: []api.Device{{DeviceID: 456, DeviceType: "ST"}, {DeviceID: 457, DeviceType: "ST"}, {DeviceID: 458, DeviceType: "HB"}}},
		{StationID: 789, Name: "Second station", Devices: []api.Device{{DeviceID: 777, DeviceType: "ST"}}},
	}
}

func selectionClient(t *testing.T, handler http.HandlerFunc) *api.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := api.New("synthetic-selection-token", api.WithBaseURL(server.URL), api.WithCacheTTL(0))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// Use Run rather than manually constructing liveSource, so the tests cover
// resolved Options, lazy startup, capability registration and archive ownership.
func connectSelectedServer(t *testing.T, options Options) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	options.Transport = serverTransport
	done := make(chan error, 1)
	go func() { done <- Run(ctx, options) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("stop selected server: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("selected server did not stop")
		}
	})
	client, err := mcp.NewClient(&mcp.Implementation{Name: "device-selection", Version: "test"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, client)
	return client
}

func TestMCPSelectedDeviceControlsDefaultStationAndCollection(t *testing.T) {
	t.Setenv("TEMPEST_THROTTLE_MS", "0")
	for _, tc := range []struct {
		name                  string
		selected, device, sid int
	}{
		{"automatic", 0, 456, 123},
		{"second station", 777, 777, 789},
		{"second Tempest in same station", 457, 457, 123},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests, discovery, deviceCalls atomic.Int32
			client := selectionClient(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/stations":
					discovery.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"stations": selectionStations()})
				case strings.HasPrefix(r.URL.Path, "/observations/station/"):
					if r.URL.Path != "/observations/station/"+strconv.Itoa(tc.sid) {
						t.Errorf("wrong default live station: %s", r.URL.Path)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"obs": []api.StationObs{{Timestamp: 1700000000, AirTemperature: new(20.0)}}})
				case r.URL.Path == "/better_forecast":
					if id := r.URL.Query().Get("station_id"); id != "123" && id != strconv.Itoa(tc.sid) {
						t.Errorf("wrong forecast station: %s", id)
					}
					_ = json.NewEncoder(w).Encode(api.Forecast{CurrentConditions: api.ForecastCurrent{Time: 1700000000}})
				case strings.HasPrefix(r.URL.Path, "/observations/device/"):
					deviceCalls.Add(1)
					if r.URL.Path != "/observations/device/"+strconv.Itoa(tc.device) {
						t.Errorf("collected from wrong device: %s", r.URL.Path)
					}
					epoch, _ := strconv.ParseInt(r.URL.Query().Get("time_start"), 10, 64)
					writeObs(w, []int64{epoch})
				default:
					http.NotFound(w, r)
				}
			})
			path := filepath.Join(t.TempDir(), "selected.sqlite")
			cs := connectSelectedServer(t, Options{Client: client, DBPath: path, DeviceID: tc.selected})
			if _, err := cs.ListTools(t.Context(), nil); err != nil {
				t.Fatal(err)
			}
			if requests.Load() != 0 {
				t.Fatal("startup or discovery contacted the API")
			}
			var conditions ConditionsOut
			callTool(t, t.Context(), cs, "current_conditions", nil, &conditions)
			wantName := "First station"
			if tc.sid == 789 {
				wantName = "Second station"
			}
			if conditions.Source != "live" || conditions.Station != wantName {
				t.Fatalf("default conditions = %+v", conditions)
			}
			var overridden ForecastOut
			callTool(t, t.Context(), cs, "forecast", map[string]any{"station_id": 123}, &overridden)
			if overridden.Station != "First station" {
				t.Fatalf("explicit station was ignored: %+v", overridden)
			}
			var explicit StationDetailsOut
			callTool(t, t.Context(), cs, "station_details", map[string]any{"station_id": 123}, &explicit)
			if explicit.StationID != 123 {
				t.Fatalf("explicit station details = %+v", explicit)
			}
			beforeDefault := discovery.Load()
			var details StationDetailsOut
			callTool(t, t.Context(), cs, "station_details", nil, &details)
			var forecast ForecastOut
			callTool(t, t.Context(), cs, "forecast", nil, &forecast)
			if details.StationID != tc.sid || forecast.Station != wantName || discovery.Load() != beforeDefault {
				t.Fatalf("override changed cached default: details=%+v forecast=%+v", details, forecast)
			}
			var backfill BackfillOut
			callTool(t, t.Context(), cs, "backfill_archive", map[string]any{"start": "2025-01-01", "end": "2025-01-02", "max_days": 1}, &backfill)
			if backfill.RowsAdded != 1 || deviceCalls.Load() != 1 {
				t.Fatalf("selected backfill = %+v, calls=%d", backfill, deviceCalls.Load())
			}
			st, err := store.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			closeOnCleanup(t, st)
			if err := st.EachArchiveObs(t.Context(), 0, 1800000000, func(o store.ArchiveObs) error {
				if o.DeviceID == nil || *o.DeviceID != int64(tc.device) {
					t.Fatalf("archive contains wrong device: %+v", o.DeviceID)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			var info StationInfoOut
			callTool(t, t.Context(), cs, "station_info", nil, &info)
			if info.StationID != tc.sid || info.Name != wantName {
				t.Fatalf("selected archive label = %+v", info)
			}
		})
	}
}

func TestMCPDeviceSelectionFailuresDoNotWriteAndCanRetry(t *testing.T) {
	t.Setenv("TEMPEST_THROTTLE_MS", "0")
	for _, failure := range []string{"unknown", "hub", "empty stations", "unavailable"} {
		t.Run(failure, func(t *testing.T) {
			var ready atomic.Bool
			var deviceCalls atomic.Int32
			const selected = 999
			client := selectionClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/stations" {
					stations := selectionStations()
					if ready.Load() {
						stations[1].Devices = []api.Device{{DeviceID: selected, DeviceType: "ST"}}
					} else {
						switch failure {
						case "hub":
							stations[1].Devices = []api.Device{{DeviceID: selected, DeviceType: "HB"}}
						case "empty stations":
							stations = []api.Station{}
						case "unavailable":
							w.WriteHeader(http.StatusUnauthorized)
							return
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"stations": stations})
					return
				}
				if strings.HasPrefix(r.URL.Path, "/observations/device/") {
					deviceCalls.Add(1)
					if r.URL.Path != "/observations/device/999" {
						t.Errorf("retried wrong device: %s", r.URL.Path)
					}
					epoch, _ := strconv.ParseInt(r.URL.Query().Get("time_start"), 10, 64)
					writeObs(w, []int64{epoch})
					return
				}
				http.NotFound(w, r)
			})
			path := filepath.Join(t.TempDir(), "unbound.sqlite")
			cs := connectSelectedServer(t, Options{Client: client, DBPath: path, DeviceID: selected})
			st, err := store.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			closeOnCleanup(t, st)
			before := selectionArchiveState(t, st)
			for _, tool := range []string{"backfill_archive", "sync_archive"} {
				result, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tool})
				if err != nil || !result.IsError {
					t.Fatalf("%s with failed selection: result=%+v error=%v", tool, result, err)
				}
			}
			if after := selectionArchiveState(t, st); !reflect.DeepEqual(after, before) || deviceCalls.Load() != 0 {
				t.Fatalf("failed selection changed archive or fetched observations: before=%v after=%v calls=%d", before, after, deviceCalls.Load())
			}
			if failure != "unavailable" {
				var stations StationsOut
				callTool(t, t.Context(), cs, "list_stations", nil, &stations)
			}
			ready.Store(true)
			var backfill BackfillOut
			callTool(t, t.Context(), cs, "backfill_archive", map[string]any{"start": "2025-01-01", "end": "2025-01-02", "max_days": 1}, &backfill)
			if backfill.RowsAdded != 1 || deviceCalls.Load() != 1 {
				t.Fatalf("selection failure was cached: %+v calls=%d", backfill, deviceCalls.Load())
			}
		})
	}
}

func selectionArchiveState(t *testing.T, st *store.Store) []store.QueryResult {
	t.Helper()
	var state []store.QueryResult
	for _, sql := range []string{"SELECT device_id, epoch FROM obs_st ORDER BY epoch", "SELECT key,value FROM meta ORDER BY key"} {
		result, err := st.Query(t.Context(), sql, 100)
		if err != nil {
			t.Fatal(err)
		}
		state = append(state, result)
	}
	return state
}

func TestMCPSelectedDevicePreservesArchiveIdentityAndOfflineReads(t *testing.T) {
	for _, tc := range []struct {
		name     string
		selected int
		live     bool
	}{
		{"different station", 777, true},
		{"unknown selected device", 999, true},
		{"archive only", 777, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "existing.sqlite")
			writer, err := store.OpenWriter(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			_, insertErr := writer.InsertObs(t.Context(), 456, []model.DeviceObs{
				{Epoch: 1700000000 - 3*3600, PressureMb: new(1018.0)},
				{Epoch: 1700000000, PressureMb: new(1012.0), AirTempC: new(10.0)},
			})
			if err := errors.Join(insertErr, writer.Close()); err != nil {
				t.Fatal(err)
			}
			var deviceCalls atomic.Int32
			options := Options{DeviceID: tc.selected, DBPath: path}
			if tc.live {
				options.Client = selectionClient(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch {
					case r.URL.Path == "/stations":
						_ = json.NewEncoder(w).Encode(map[string]any{"stations": selectionStations()})
					case r.URL.Path == "/observations/station/789":
						_ = json.NewEncoder(w).Encode(map[string]any{"obs": []api.StationObs{{Timestamp: 1700000000, AirTemperature: new(20.0)}}})
					case strings.HasPrefix(r.URL.Path, "/observations/device/"):
						deviceCalls.Add(1)
						writeObs(w, nil)
					default:
						http.NotFound(w, r)
					}
				})
			}
			cs := connectSelectedServer(t, options)
			st, err := store.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			closeOnCleanup(t, st)
			before := selectionArchiveState(t, st)
			var info StationInfoOut
			callTool(t, t.Context(), cs, "station_info", nil, &info)
			if info.Name != "" || info.StationID != 0 || info.Observations != 2 {
				t.Fatalf("mislabeled archive: %+v", info)
			}
			var conditions ConditionsOut
			callTool(t, t.Context(), cs, "current_conditions", nil, &conditions)
			if tc.live && tc.selected == 777 {
				if conditions.Source != "live" || conditions.Station != "Second station" || conditions.PressureTrend3hInHg != nil {
					t.Fatalf("mismatched source enrichment: %+v", conditions)
				}
			} else if conditions.Source != "archive" || conditions.Station != "" || conditions.TempC == nil || *conditions.TempC != 10 {
				t.Fatalf("archive fallback unavailable or mislabeled: %+v", conditions)
			}
			if tc.live {
				for _, tool := range []string{"backfill_archive", "sync_archive"} {
					result, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tool})
					if err != nil || !result.IsError {
						t.Fatalf("%s accepted wrong archive device: result=%+v error=%v", tool, result, err)
					}
				}
			}
			if after := selectionArchiveState(t, st); !reflect.DeepEqual(after, before) || deviceCalls.Load() != 0 {
				t.Fatalf("failed selection/mismatch changed archive: before=%v after=%v calls=%d", before, after, deviceCalls.Load())
			}
		})
	}
}

func TestSelectedDeviceResolutionCancellationAndConcurrency(t *testing.T) {
	var requests atomic.Int32
	client := selectionClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"stations": selectionStations()})
	})
	live := &liveSource{client: client, selectedDeviceID: 777}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := live.resolveStation(canceled); !errors.Is(err, context.Canceled) || requests.Load() != 0 {
		t.Fatalf("canceled resolution: error=%v requests=%d", err, requests.Load())
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			device, err := live.resolveDevice(t.Context())
			if err != nil || device != 777 {
				t.Errorf("concurrent resolution: device=%d error=%v", device, err)
			}
		})
	}
	wg.Wait()
	before := requests.Load()
	if _, err := live.resolveStation(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cached resolution ignored cancellation: %v", err)
	}
	station, err := live.resolveStation(t.Context())
	if err != nil || station.StationID != 789 || requests.Load() != before {
		t.Fatalf("successful resolution was not cached: station=%+v error=%v", station, err)
	}
}

func TestRunRejectsNegativeDeviceBeforeArchiveInitialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "must-not-exist.sqlite")
	if err := Run(t.Context(), Options{DeviceID: -1, DBPath: path}); err == nil || !strings.Contains(err.Error(), "device id") {
		t.Fatalf("negative option: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("negative option initialized archive: %v", err)
	}
}
