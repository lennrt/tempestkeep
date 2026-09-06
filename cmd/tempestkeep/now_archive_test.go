package main

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

const nowStationsJSON = `{"stations":[{"station_id":123,"name":"Test Station","latitude":0,"longitude":0,"timezone":"UTC","station_meta":{"elevation":10},"devices":[{"device_id":456,"device_type":"ST","serial_number":"ST-TEST"}]}]}`

const nowObsJSON = `{"obs":[{"timestamp":1700000000,"air_temperature":20.5,"relative_humidity":45,"sea_level_pressure":1013,"wind_avg":2,"wind_gust":4,"wind_direction":180,"uv":5,"solar_radiation":500,"feels_like":21,"dew_point":8,"precip_accum_local_day":1,"lightning_strike_count_last_1hr":2,"lightning_strike_last_distance":10}]}`

func TestFillArchiveRainTodaySumsIntervals(t *testing.T) {
	// Keep both intervals in the requested day, even when CI runs at midnight.
	now := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.Local)
	path := filepath.Join(t.TempDir(), "rain.sqlite")
	w, err := store.OpenWriter(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	rain := func(v float64) *float64 { return &v }
	obs := []model.DeviceObs{
		{Epoch: now.Add(-2 * time.Minute).Unix(), RainMm: rain(12.7)},
		{Epoch: now.Add(-time.Minute).Unix(), RainMm: rain(12.7)},
	}
	if _, err := w.InsertObs(context.Background(), 1, obs); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, st)

	d := dashboard{obsTime: time.Unix(obs[1].Epoch, 0)}
	if err := fillArchiveRainToday(context.Background(), st, &d, now); err != nil {
		t.Fatal(err)
	}
	if d.rainTodayIn == nil || math.Abs(*d.rainTodayIn-1) > 1e-9 {
		t.Fatalf("rainTodayIn = %v, want 1 inch", d.rainTodayIn)
	}
}

func TestFillArchiveRainTodayIgnoresStaleArchive(t *testing.T) {
	// The most recent interval can belong to yesterday just after midnight.
	now := time.Date(2026, time.September, 5, 0, 0, 30, 0, time.Local)
	d := dashboard{obsTime: now.Add(-time.Minute)}
	if err := fillArchiveRainToday(context.Background(), nil, &d, now); err != nil {
		t.Fatal(err)
	}
	if d.rainTodayIn != nil {
		t.Fatalf("stale archive reported today's rain: %v", d.rainTodayIn)
	}
}

func TestResolveNowConfigDefersLiveLookup(t *testing.T) {
	t.Setenv("TEMPEST_API_BASE", "http://127.0.0.1:1")
	t.Setenv("TEMPEST_DB", "")
	t.Setenv("TEMPEST_TOKEN", "synthetic-test-token")
	cfg, err := resolveNowConfig(t.Context(), "")
	if err != nil {
		t.Fatalf("configuration should not contact the live API: %v", err)
	}
	if cfg.live == nil || cfg.live.client == nil {
		t.Fatal("live source was not configured")
	}
}

func TestResolveNowConfigRejectsUnavailableConfiguredArchive(t *testing.T) {
	t.Setenv("TEMPEST_TOKEN", "synthetic-test-token")
	t.Setenv("TEMPEST_DB", filepath.Join(t.TempDir(), "missing.sqlite"))
	if _, err := resolveNowConfig(t.Context(), ""); err == nil {
		t.Fatal("resolveNowConfig accepted an unavailable configured archive")
	}
}

func TestNowLoadReportsOptionalForecastFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/stations":
			_, _ = w.Write([]byte(nowStationsJSON))
		case strings.HasPrefix(r.URL.Path, "/observations/station/"):
			_, _ = w.Write([]byte(nowObsJSON))
		case r.URL.Path == "/better_forecast":
			http.Error(w, "unavailable", http.StatusBadRequest)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("TEMPEST_API_BASE", server.URL)

	client, err := newAPIClient("synthetic-test-token")
	if err != nil {
		t.Fatal(err)
	}
	d, err := (nowConfig{live: &nowLiveSource{client: client}}).load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if d.tempF == nil || d.note != "live forecast unavailable" {
		t.Fatalf("dashboard = %+v, want live data with a forecast warning", d)
	}
}

func TestNowLoadCancelsForecastAfterObservationFailure(t *testing.T) {
	forecastStarted := make(chan struct{})
	forecastCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/stations":
			_, _ = w.Write([]byte(nowStationsJSON))
		case strings.HasPrefix(r.URL.Path, "/observations/station/"):
			select {
			case <-forecastStarted:
				_, _ = w.Write([]byte(`{"obs":[]}`))
			case <-r.Context().Done():
			}
		case r.URL.Path == "/better_forecast":
			close(forecastStarted)
			<-r.Context().Done()
			close(forecastCanceled)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("TEMPEST_API_BASE", server.URL)

	client, err := newAPIClient("synthetic-test-token")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err := (nowConfig{live: &nowLiveSource{client: client}}).load(ctx); err == nil {
		t.Fatal("load accepted a missing observation")
	}
	select {
	case <-forecastCanceled:
	case <-ctx.Done():
		t.Fatal("forecast request was not canceled")
	}
}

func TestNowLoadAnnouncesArchiveFallback(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "fallback.sqlite")
	writer, err := store.OpenWriter(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, writer)
	if _, err := writer.InsertObs(ctx, 456, []model.DeviceObs{{Epoch: 1700000000, AirTempC: new(20.0)}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, st)

	for _, tc := range []struct {
		name                 string
		live, resolveStation bool
	}{
		{"archive only", false, false},
		{"station lookup fails", true, false},
		{"observation fails", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := nowConfig{store: st}
			if tc.live {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/stations" && tc.resolveStation {
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(nowStationsJSON))
						return
					}
					http.Error(w, "synthetic-sensitive-body", http.StatusBadRequest)
				}))
				t.Cleanup(srv.Close)
				client, err := api.New("synthetic-test-token", api.WithBaseURL(srv.URL))
				if err != nil {
					t.Fatal(err)
				}
				cfg.live = &nowLiveSource{client: client}
			}
			d, err := cfg.load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if d.source != "archive" || d.tempF == nil || *d.tempF != 68 {
				t.Fatalf("missing archived reading: %+v", d)
			}
			if strings.Contains(d.note, "live data unavailable") != tc.live {
				t.Fatalf("incorrect fallback note: %q", d.note)
			}
			if !strings.Contains(d.note, "pressure is station, not sea-level") {
				t.Fatalf("lost archive limitation: %q", d.note)
			}
			var output bytes.Buffer
			if err := writeNowJSON(&output, d); err != nil {
				t.Fatal(err)
			}
			var decoded nowJSON
			if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Note != d.note {
				t.Fatalf("JSON note = %q, want %q", decoded.Note, d.note)
			}
			if tc.live && !strings.Contains(renderDashboard(d, time.Now(), ""), "live data unavailable") {
				t.Fatal("dashboard hides fallback warning")
			}
			if strings.Contains(output.String(), "synthetic-sensitive-body") {
				t.Fatal("JSON leaked upstream error body")
			}
		})
	}
}
