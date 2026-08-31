package main

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

const nowStationsJSON = `{"stations":[{"station_id":123,"name":"Test Station","latitude":0,"longitude":0,"timezone":"UTC","station_meta":{"elevation":10},"devices":[{"device_id":456,"device_type":"ST","serial_number":"ST-TEST"}]}]}`

const nowObsJSON = `{"obs":[{"timestamp":1700000000,"air_temperature":20.5,"relative_humidity":45,"sea_level_pressure":1013,"wind_avg":2,"wind_gust":4,"wind_direction":180,"uv":5,"solar_radiation":500,"feels_like":21,"dew_point":8,"precip_accum_local_day":1,"lightning_strike_count_last_1hr":2,"lightning_strike_last_distance":10}]}`

func TestFillArchiveRainTodaySumsIntervals(t *testing.T) {
	now := time.Now().Truncate(time.Second)
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
	d := dashboard{obsTime: time.Now().AddDate(0, 0, -1)}
	if err := fillArchiveRainToday(context.Background(), nil, &d, time.Now()); err != nil {
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
