package api

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const stationsJSON = `{
  "stations": [{
    "station_id": 123,
    "name": "Test Station",
    "latitude": 0,
    "longitude": 0,
    "timezone": "America/Los_Angeles",
    "station_meta": {"elevation": 300.5},
    "devices": [{"device_id": 456, "device_type": "ST", "serial_number": "ST-0001"}]
  }]
}`

const obsJSON = `{
  "obs": [{
    "timestamp": 1700000000,
    "air_temperature": 20.5,
    "relative_humidity": 45,
    "sea_level_pressure": 1013.0,
    "wind_avg": 2.0,
    "wind_gust": 4.0,
    "wind_direction": 180,
    "uv": 5,
    "solar_radiation": 500,
    "feels_like": 21.0,
    "dew_point": 8.0,
    "precip_accum_local_day": 1.0,
    "lightning_strike_count_last_1hr": 2,
    "lightning_strike_last_distance": 10
  }]
}`

const forecastJSON = `{
  "current_conditions": {"time": 1700000000, "conditions": "Clear", "icon": "clear-day", "air_temperature": 20.0, "feels_like": 20.0, "relative_humidity": 40},
  "forecast": {
    "daily": [{"day_start_local": 1700000000, "conditions": "Clear", "icon": "clear-day", "sunrise": 1700010000, "sunset": 1700050000, "air_temp_high": 25, "air_temp_low": 10, "precip_probability": 0}],
    "hourly": [{"time": 1700000000, "conditions": "Clear", "icon": "clear-day", "air_temperature": 18, "feels_like": 18, "relative_humidity": 50, "precip_probability": 0, "wind_avg": 2, "wind_gust": 4, "wind_direction": 90, "uv": 3}]
  }
}`

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	return mustClient(t, "test-token", WithBaseURL(baseURL))
}

func mustClient(t *testing.T, token string, options ...Option) *Client {
	t.Helper()
	client, err := New(token, options...)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func writeTestJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func TestStations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/stations" {
			t.Errorf("path = %q, want /stations", r.URL.Path)
		}
		if r.URL.Query().Get("token") != "test-token" {
			t.Errorf("token not forwarded: %q", r.URL.Query().Get("token"))
		}
		writeTestJSON(w, stationsJSON)
	}))
	defer srv.Close()

	sts, err := newTestClient(t, srv.URL).Stations(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(sts) != 1 {
		t.Fatalf("got %d stations, want 1", len(sts))
	}
	s := sts[0]
	if s.StationID != 123 || s.Name != "Test Station" {
		t.Errorf("unexpected station %+v", s)
	}
	if s.StationMeta.Elevation != 300.5 {
		t.Errorf("elevation = %v, want 300.5", s.StationMeta.Elevation)
	}
	if len(s.Devices) != 1 || s.Devices[0].DeviceType != "ST" || s.Devices[0].SerialNumber != "ST-0001" {
		t.Errorf("unexpected devices %+v", s.Devices)
	}
}

func TestRejectsOversizedResponseFromContentLength(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(maxResponseSize+1))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	_, err := newTestClient(t, ts.URL).Stations(t.Context())
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("Stations error = %v, want response-size error", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	if got := parseRetryAfter("12", now); got != 12*time.Second {
		t.Errorf("seconds Retry-After = %s", got)
	}
	if got := parseRetryAfter(now.Add(30*time.Second).Format(http.TimeFormat), now); got != 30*time.Second {
		t.Errorf("date Retry-After = %s", got)
	}
	if got := parseRetryAfter("garbage", now); got != 0 {
		t.Errorf("invalid Retry-After = %s", got)
	}
}

func TestLatestStationObsForcesSIUnits(t *testing.T) {
	// Assert the request shape inside the handler (which runs synchronously
	// during the call) to avoid sharing state across goroutines.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/observations/station/") {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("units_temp") != "c" || q.Get("units_wind") != "mps" {
			t.Errorf("SI unit params not sent: %v", q)
		}
		writeTestJSON(w, obsJSON)
	}))
	defer srv.Close()

	obs, err := newTestClient(t, srv.URL).LatestStationObs(t.Context(), 123)
	if err != nil {
		t.Fatal(err)
	}
	if obs.AirTemperature == nil || *obs.AirTemperature != 20.5 {
		t.Errorf("air_temperature = %v, want 20.5", obs.AirTemperature)
	}
	if obs.LightningLast1hr == nil || *obs.LightningLast1hr != 2 {
		t.Errorf("lightning_strike_count_last_1hr = %v, want 2", obs.LightningLast1hr)
	}
}

func TestBetterForecast(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/better_forecast" {
			t.Errorf("path = %q, want /better_forecast", r.URL.Path)
		}
		if r.URL.Query().Get("station_id") != "123" {
			t.Errorf("station_id = %q, want 123", r.URL.Query().Get("station_id"))
		}
		writeTestJSON(w, forecastJSON)
	}))
	defer srv.Close()

	f, err := newTestClient(t, srv.URL).BetterForecast(t.Context(), 123)
	if err != nil {
		t.Fatal(err)
	}
	if f.CurrentConditions.Conditions != "Clear" {
		t.Errorf("conditions = %q", f.CurrentConditions.Conditions)
	}
	if len(f.Forecast.Daily) != 1 || f.Forecast.Daily[0].AirTempHigh == nil || *f.Forecast.Daily[0].AirTempHigh != 25 {
		t.Errorf("daily forecast decoded wrong: %+v", f.Forecast.Daily)
	}
	if len(f.Forecast.Hourly) != 1 || f.Forecast.Hourly[0].WindAvg == nil || *f.Forecast.Hourly[0].WindAvg != 2 {
		t.Errorf("hourly forecast decoded wrong: %+v", f.Forecast.Hourly)
	}
}

func TestBetterForecastRejectsInvalidFields(t *testing.T) {
	cases := []string{
		`{"current_conditions":{"time":1700000000,"relative_humidity":101},"forecast":{"daily":[],"hourly":[]}}`,
		`{"current_conditions":{"time":1700000000},"forecast":{"daily":[{"day_start_local":-1}],"hourly":[]}}`,
		`{"current_conditions":{"time":1700000000},"forecast":{"daily":[],"hourly":[{"time":1700000000,"wind_avg":-1}]}}`,
		`{"current_conditions":{"time":1700000000},"forecast":{"daily":[{"day_start_local":1700000000,"precip_probability":101}],"hourly":[]}}`,
	}
	for _, body := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeTestJSON(w, body)
		}))
		_, err := newTestClient(t, srv.URL).BetterForecast(t.Context(), 123)
		srv.Close()
		if !errors.Is(err, ErrMalformedResponse) {
			t.Errorf("body %s: error = %v, want ErrMalformedResponse", body, err)
		}
	}
}

func TestCacheHit(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		writeTestJSON(w, stationsJSON)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	c.ttl = time.Minute // ensure caching is on
	ctx := t.Context()
	if _, err := c.Stations(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Stations(ctx); err != nil {
		t.Fatal(err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("server hits = %d, want 1 (second call should be cached)", n)
	}
}

func TestCacheDisabled(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		writeTestJSON(w, stationsJSON)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	c.ttl = 0 // disabled
	ctx := t.Context()
	_, _ = c.Stations(ctx)
	_, _ = c.Stations(ctx)
	if n := hits.Load(); n != 2 {
		t.Errorf("server hits = %d, want 2 (caching disabled)", n)
	}
}

func TestRetryOnTransientStatus(t *testing.T) {
	// 429 then 500 then success: the client must retry through both and succeed.
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch hits.Add(1) {
		case 1:
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			writeTestJSON(w, stationsJSON)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	c.retry = RetryPolicy{MaxAttempts: defaultMaxAttempts, BaseWait: time.Millisecond, MaxWait: time.Millisecond}
	if _, err := c.Stations(t.Context()); err != nil {
		t.Fatalf("Stations after transient failures: %v", err)
	}
	if n := hits.Load(); n != 3 {
		t.Errorf("server hits = %d, want 3 (two retries then success)", n)
	}
}

func TestNoRetryOnPermanentStatus(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	c.retry = RetryPolicy{MaxAttempts: defaultMaxAttempts, BaseWait: time.Millisecond, MaxWait: time.Millisecond}
	if _, err := c.Stations(t.Context()); err == nil {
		t.Fatal("expected an error for 404")
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("server hits = %d, want 1 (404 is not retryable)", n)
	}
}

func TestRetryGivesUpAfterMaxAttempts(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	c.retry = RetryPolicy{MaxAttempts: defaultMaxAttempts, BaseWait: time.Millisecond, MaxWait: time.Millisecond}
	if _, err := c.Stations(t.Context()); err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if n := hits.Load(); n != defaultMaxAttempts {
		t.Errorf("server hits = %d, want %d", n, defaultMaxAttempts)
	}
}

func TestRetryStopsOnCancellation(t *testing.T) {
	firstAttempt := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case <-firstAttempt:
		default:
			close(firstAttempt)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	client.retry = RetryPolicy{MaxAttempts: defaultMaxAttempts, BaseWait: time.Minute, MaxWait: time.Minute}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	done := make(chan error, 1)
	go func() {
		_, err := client.Stations(ctx)
		done <- err
	}()
	<-firstAttempt
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Stations error = %v, want context cancellation", err)
	}
}

func TestTransportErrorRedactsToken(t *testing.T) {
	// Point at a closed port so http.Do fails with a *url.Error quoting the
	// request URL. The secret must not survive into the error message.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	srv.Close() // now nothing listens on srv.URL

	c := newTestClient(t, srv.URL)
	c.retry = RetryPolicy{MaxAttempts: defaultMaxAttempts, BaseWait: time.Millisecond, MaxWait: time.Millisecond}
	_, err := c.Stations(t.Context())
	if err == nil {
		t.Fatal("expected a transport error against a closed server")
	}
	if strings.Contains(err.Error(), "test-token") {
		t.Errorf("error leaks the token: %v", err)
	}
	if !errors.Is(err, ErrTransport) {
		t.Errorf("error = %v, want ErrTransport", err)
	}
	if strings.Contains(err.Error(), srv.URL) {
		t.Errorf("error leaks the request URL: %v", err)
	}
}

func TestDeviceObservationsNotCached(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		writeTestJSON(w, deviceObsJSON)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	c.ttl = time.Minute // caching on; device windows must bypass it
	ctx := t.Context()
	for range 2 {
		if _, err := c.DeviceObservations(ctx, 456, 1700000000, 1700000300); err != nil {
			t.Fatal(err)
		}
	}
	if n := hits.Load(); n != 2 {
		t.Errorf("server hits = %d, want 2 (historical windows are never cached)", n)
	}
}

func TestCachePutEvictsExpired(t *testing.T) {
	c := mustClient(t, "tok")
	c.ttl = time.Minute
	c.cache["stale"] = cachedBody{body: []byte("x"), exp: time.Now().Add(-time.Second)}
	c.cachePut("fresh", []byte("y"))
	if _, ok := c.cache["stale"]; ok {
		t.Error("expired entry survived cachePut; the cache would grow without bound")
	}
	if _, ok := c.cache["fresh"]; !ok {
		t.Error("fresh entry missing after cachePut")
	}
}

func TestUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL).Stations(t.Context())
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("err = %v, want an unauthorized error", err)
	}
}

// deviceObsJSON is two raw obs_st rows. The second has a null wind_avg (index 2)
// to exercise null handling, and a precip_type of 1 (index 13).
const deviceObsJSON = `{
  "device_id": 456,
  "type": "obs_st",
  "obs": [
    [1700000000, 0.2, 1.1, 2.3, 180, 3, 1013.2, 20.5, 45, 1000, 5.0, 450, 0.0, 0, 10, 2, 2.6, 1],
    [1700000060, 0.3, null, 2.9, 200, 3, 1013.0, 21.0, 44, 1100, 5.2, 460, 0.0, 1, 10, 0, 2.6, 1]
  ]
}`

func TestDeviceObservations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/observations/device/456" {
			t.Errorf("path = %q, want /observations/device/456", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("time_start") != "1700000000" || q.Get("time_end") != "1700000300" {
			t.Errorf("time range not forwarded: start=%q end=%q", q.Get("time_start"), q.Get("time_end"))
		}
		if q.Get("units_temp") != "c" {
			t.Errorf("SI units not sent: %v", q)
		}
		writeTestJSON(w, deviceObsJSON)
	}))
	defer srv.Close()

	obs, err := newTestClient(t, srv.URL).DeviceObservations(t.Context(), 456, 1700000000, 1700000300)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 2 {
		t.Fatalf("got %d observations, want 2", len(obs))
	}
	o := obs[0]
	if o.Epoch != 1700000000 {
		t.Errorf("epoch = %d, want 1700000000", o.Epoch)
	}
	if o.AirTempC == nil || *o.AirTempC != 20.5 {
		t.Errorf("air_temp_c = %v, want 20.5", o.AirTempC)
	}
	if o.PrecipType == nil || *o.PrecipType != 0 {
		t.Errorf("precip_type = %v, want 0 (index 13)", o.PrecipType)
	}
	if o.ReportIntervalMin == nil || *o.ReportIntervalMin != 1 {
		t.Errorf("report_interval_min = %v, want 1 (last index, 17)", o.ReportIntervalMin)
	}
	// A JSON null decodes to a nil pointer, distinct from a real zero.
	if obs[1].WindAvg != nil {
		t.Errorf("wind_avg[1] = %v, want nil (JSON null)", obs[1].WindAvg)
	}
	if obs[1].PrecipType == nil || *obs[1].PrecipType != 1 {
		t.Errorf("precip_type[1] = %v, want 1", obs[1].PrecipType)
	}
}

func TestDeviceObservationsWindowTooLarge(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		writeTestJSON(w, deviceObsJSON)
	}))
	defer srv.Close()

	start := int64(1700000000)
	end := start + int64(6*24*3600) // 6 days > the 5-day one-minute limit
	if _, err := newTestClient(t, srv.URL).DeviceObservations(t.Context(), 456, start, end); err == nil {
		t.Fatal("expected an error for a window wider than 5 days")
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("server hits = %d, want 0 (guard must reject before any request)", n)
	}
}

func TestDeviceObservationsStartAfterEnd(t *testing.T) {
	if _, err := mustClient(t, "test-token").DeviceObservations(t.Context(), 456, 200, 100); err == nil {
		t.Error("expected an error when start is after end")
	}
}

func TestDeviceObservationsRejectsEpochOverflowBeforeRequest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		writeTestJSON(w, deviceObsJSON)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL).DeviceObservations(t.Context(), 456, 0, math.MaxInt64)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("error = %v, want ErrInvalidArgument", err)
	}
	if hits.Load() != 0 {
		t.Fatal("invalid epoch reached the HTTP boundary")
	}
}

func TestStationsRejectsSemanticCountLimit(t *testing.T) {
	stations := make([]Station, maxStations+1)
	for index := range stations {
		stations[index] = Station{StationID: index + 1}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"stations": stations}); err != nil {
			t.Errorf("encode stations: %v", err)
		}
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL).Stations(t.Context())
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("error = %v, want ErrMalformedResponse", err)
	}
}

func TestPickTempestDevice(t *testing.T) {
	// Prefers the ST (Tempest) device even when another device comes first.
	stations := []Station{{
		StationID: 1, Name: "Home",
		Devices: []Device{
			{DeviceID: 10, DeviceType: "HB"},
			{DeviceID: 11, DeviceType: "ST"},
		},
	}}
	if s, dev, ok := PickTempestDevice(stations); !ok || dev != 11 || s.StationID != 1 {
		t.Errorf("PickTempestDevice = station %v dev %d ok %v, want station 1 dev 11", s, dev, ok)
	} else {
		stations[0].Devices[1].DeviceType = "changed"
		if s.Devices[1].DeviceType != "ST" {
			t.Error("returned station retained the caller-owned Devices slice")
		}
	}

	// With no Tempest, falls back to the first device of the first station.
	legacy := []Station{{StationID: 2, Devices: []Device{{DeviceID: 20, DeviceType: "AR"}}}}
	if _, dev, ok := PickTempestDevice(legacy); !ok || dev != 20 {
		t.Errorf("legacy fallback dev = %d ok %v, want 20/true", dev, ok)
	}

	// No devices at all: not ok.
	if _, _, ok := PickTempestDevice([]Station{{StationID: 3}}); ok {
		t.Error("empty station should report ok=false")
	}
}

func TestBaseURLOverride(t *testing.T) {
	c := mustClient(t, "tok", WithBaseURL("http://mock.test/rest/"))
	if got := c.baseURL.String(); got != "http://mock.test/rest" {
		t.Errorf("baseURL = %q, want the overridden host without a trailing slash", got)
	}
}
