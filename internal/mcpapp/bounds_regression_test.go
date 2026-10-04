package mcpapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDailySummaryStartWithoutEndStopsNow(t *testing.T) {
	before := time.Now()
	startDate := before.AddDate(0, 0, -1).Format(time.DateOnly)
	_, end, err := resolveRange(DailySummaryArgs{Start: startDate})
	if err != nil {
		t.Fatal(err)
	}
	if end < before.Unix() || end > time.Now().Unix() {
		t.Fatalf("end = %s, want the current time", time.Unix(end, 0))
	}
	if _, _, err := resolveRange(DailySummaryArgs{Start: before.AddDate(0, 0, 2).Format(time.DateOnly)}); err == nil {
		t.Fatal("future start without an end must fail")
	}
}

func TestDailySummaryIncludesWholeDSTDay(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	previous := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = previous })
	for _, tc := range []struct {
		date  string
		hours int64
	}{
		{"2024-03-10", 23},
		{"2024-11-03", 25},
	} {
		t.Run(tc.date, func(t *testing.T) {
			start, end, err := resolveRange(DailySummaryArgs{Start: tc.date, End: tc.date})
			if err != nil {
				t.Fatal(err)
			}
			if end-start+1 != tc.hours*3600 {
				t.Fatalf("day contains %d seconds, want %d", end-start+1, tc.hours*3600)
			}
			if localTimeStr(end)[11:19] != "23:59:59" {
				t.Fatalf("end = %s, want last second of the requested day", localTimeStr(end))
			}
		})
	}
}

func TestForecastUsesRequestedStation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var forecastCalls atomic.Int64
	stations := []api.Station{
		{StationID: 123, Name: "Default station", Devices: []api.Device{{DeviceID: 456, DeviceType: "ST"}}},
		{StationID: 789, Name: "Requested station"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/stations":
			_ = json.NewEncoder(w).Encode(map[string]any{"stations": stations})
		case "/better_forecast":
			forecastCalls.Add(1)
			if r.URL.Query().Get("station_id") != "789" {
				t.Errorf("forecast requested station %q", r.URL.Query().Get("station_id"))
			}
			_ = json.NewEncoder(w).Encode(api.Forecast{CurrentConditions: api.ForecastCurrent{Time: 1700000000}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TEMPEST_API_BASE", srv.URL)
	t.Setenv("TEMPEST_CACHE_TTL", "0")
	cs := connectLiveServer(t, ctx, "synthetic-token")
	var forecast ForecastOut
	callTool(t, ctx, cs, "forecast", map[string]any{"station_id": 789}, &forecast)
	if forecast.Station != "Requested station" {
		t.Fatalf("forecast station = %q", forecast.Station)
	}
	var details StationDetailsOut
	callTool(t, ctx, cs, "station_details", map[string]any{"station_id": 789}, &details)
	if details.Name != forecast.Station || details.StationID != 789 {
		t.Fatalf("forecast and details disagree: %+v / %+v", forecast, details)
	}
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "forecast", Arguments: map[string]any{"station_id": 999}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || forecastCalls.Load() != 1 {
		t.Fatalf("unknown station: error=%v, forecast requests=%d", result.IsError, forecastCalls.Load())
	}
}

func TestForecastExplicitStationDoesNotRequireTempestDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/stations" {
			_, _ = w.Write([]byte(`{"stations":[{"station_id":789,"name":"Requested station","devices":[]}]}`))
			return
		}
		if r.URL.Path == "/better_forecast" {
			_ = json.NewEncoder(w).Encode(api.Forecast{CurrentConditions: api.ForecastCurrent{Time: 1700000000}})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TEMPEST_API_BASE", srv.URL)
	t.Setenv("TEMPEST_CACHE_TTL", "0")
	cs := connectLiveServer(t, ctx, "synthetic-token")
	var forecast ForecastOut
	callTool(t, ctx, cs, "forecast", map[string]any{"station_id": 789}, &forecast)
	if !strings.Contains(forecast.Station, "Requested") {
		t.Fatalf("station = %q", forecast.Station)
	}
}

func TestAutoBucketRespectsInclusiveAlignedPointCap(t *testing.T) {
	for _, tc := range []struct {
		start, end int64
		points     int
	}{
		{0, 288 * 60, 288},
		{59, 2000 * 60, 2000},
		{1700000000, 1700086400, 288},
		{1700000000, 1700086400, 1},
		{0, 86400 - 1, 1},
	} {
		bucket := autoBucket(tc.start, tc.end, tc.points)
		if count := tc.end/bucket - tc.start/bucket + 1; count > int64(tc.points) {
			t.Fatalf("range %d..%d returned %d buckets of %d seconds, cap=%d", tc.start, tc.end, count, bucket, tc.points)
		}
		if bucket%60 != 0 || bucket < 60 {
			t.Fatalf("bucket=%d, want a positive whole-minute interval", bucket)
		}
	}
}

func TestAutoBucketChoosesSmallestValidMinuteWidth(t *testing.T) {
	// Compare the optimized calculation with an exhaustive search for modest
	// ranges, including partial buckets and intervals that request one point.
	for start := int64(0); start < 2000; start += 137 {
		for span := int64(0); span < 2500; span += 211 {
			end := start + span
			for points := 1; points <= 5; points++ {
				want := int64(60)
				for end/want-start/want+1 > int64(points) {
					want += 60
				}
				if got := autoBucket(start, end, points); got != want {
					t.Fatalf("autoBucket(%d,%d,%d)=%d, smallest valid width=%d", start, end, points, got, want)
				}
			}
		}
	}
}

func TestRequestedBucketChecksItsOwnAlignment(t *testing.T) {
	// 240-second buckets fit this range in one point. A wider, 300-second
	// bucket straddles its boundary, so comparing against the minimum alone
	// cannot establish that a requested width fits the cap.
	const start, end = int64(240), int64(360)
	if minimum := autoBucket(start, end, 1); minimum != 240 {
		t.Fatalf("minimum bucket=%d, want 240", minimum)
	}
	if got := fitSeriesBucket(start, end, 1, 300); got != 420 {
		t.Fatalf("requested bucket=%d, want 420", got)
	}
}

func TestObservationToolAcceptsHardCapBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cs := connectArchiveServer(t, ctx, makeTestArchive(t))
	var result GetObservationsOut
	callTool(t, ctx, cs, "get_observations", map[string]any{
		"start": "2023-01-01", "end": "2023-02-19", "max_points": 2000,
	}, &result)
	if len(result.Points) > 2000 {
		t.Fatalf("returned %d points", len(result.Points))
	}
}
