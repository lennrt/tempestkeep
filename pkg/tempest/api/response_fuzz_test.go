package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

// responseFuzzTransport exercises the production HTTP body limit, preflight,
// typed decoding, and endpoint validators without opening a network connection.
type responseFuzzTransport struct{ body []byte }

func (r responseFuzzTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": {"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(r.body)),
		ContentLength: int64(len(r.body)), Request: request,
	}, nil
}

func FuzzEndpointResponses(f *testing.F) {
	for endpoint, body := range []string{stationsJSON, obsJSON, deviceObsJSON, forecastJSON} {
		f.Add(uint8(endpoint), []byte(body))
	}
	for endpoint := range uint8(4) {
		for _, body := range []string{
			`null`, `[]`, `true`, `42`, `"fuzz-private-response"`, `{}`,
			`{"status":{"status_code":"fuzz-private-response"},"obs":[],"stations":[]}`,
			`{"status":{"status_code":2,"status_message":"fuzz-private-response"},"obs":[],"stations":[]}`,
			`{"status":null,"obs":null,"stations":null,"forecast":null}`,
		} {
			f.Add(endpoint, []byte(body))
		}
	}
	array := func(item string, count int) string {
		return "[" + strings.TrimSuffix(strings.Repeat(item+",", count), ",") + "]"
	}
	for _, seed := range []struct {
		endpoint uint8
		body     string
	}{
		{0, `{"stations":[{"station_id":1,"devices":[{"device_id":2,"device_type":"ST"}]}]}`},
		{0, `{"stations":[{"station_id":1,"latitude":91}]}`},
		{0, `{"stations":` + array(`{}`, maxStations+1) + `,"stations":[]}`},
		{0, `{"STATIONS":` + array(`{}`, maxStations+1) + `,"stations":[]}`},
		{0, `{"ſtations":` + array(`{}`, maxStations+1) + `,"stations":[]}`},
		{0, `{"stations":[{"devices":` + array(`{}`, maxDevicesPerStation+1) + `,"devices":[]}]}`},
		{1, `{"obs":[{"timestamp":1700000000,"relative_humidity":101}]}`},
		{1, `{"obs":` + array(`{}`, maxStationObs+1) + `,"obs":[]}`},
		{2, `{"obs":[[1700000000,null,null,null,11.25]]}`},
		{2, `{"obs":[[1699999999]]}`},
		{2, `{"device_id":457,"type":"obs_st","obs":[]}`},
		{2, `{"type":"fuzz-private-response","obs":[]}`},
		{2, `{"obs":[` + array(`0`, model.DeviceObsFields+5) + `]}`},
		{2, `{"obs":` + array(`[1700000000]`, maxDeviceObs+1) + `}`},
		{3, `{"current_conditions":{"time":1700000000,"relative_humidity":101}}`},
		{3, `{"forecast":{"daily":` + array(`{}`, maxDailyForecasts+1) + `,"daily":[]}}`},
		{3, `{"forecast":{"hourly":` + array(`{}`, maxHourlyForecasts+1) + `},"forecast":{"daily":[]}}`},
	} {
		f.Add(seed.endpoint, []byte(seed.body))
	}

	f.Fuzz(func(t *testing.T, endpoint uint8, body []byte) {
		// Keep fuzz iterations cheap; fetch's separate limit tests cover 8 MiB.
		// This still fits every collection limit plus one, including device rows.
		if len(body) > 128<<10 {
			return
		}
		client, err := New("fuzz-private-token",
			WithHTTPClient(&http.Client{Transport: responseFuzzTransport{body: body}}),
			WithBaseURL("https://fuzz-private.invalid/rest"), WithCacheTTL(0))
		if err != nil {
			t.Fatal(err)
		}
		var result any
		switch endpoint % 4 {
		case 0:
			var stations []Station
			stations, err = client.Stations(t.Context())
			if err != nil && stations != nil {
				t.Fatal("failed response returned partial stations")
			}
			if len(stations) > maxStations {
				t.Fatal("accepted oversized station collection")
			}
			for _, station := range stations {
				if len(station.Devices) > maxDevicesPerStation {
					t.Fatal("accepted oversized device collection")
				}
			}
			result = stations
		case 1:
			var observation *StationObs
			observation, err = client.LatestStationObs(t.Context(), 123)
			if err != nil && observation != nil {
				t.Fatal("failed response returned a station observation")
			}
			if err == nil && (observation == nil || observation.Timestamp <= 0 || observation.Timestamp > model.MaxEpochSeconds) {
				t.Fatal("accepted invalid station observation timestamp")
			}
			result = observation
		case 2:
			var observations []model.DeviceObs
			observations, err = client.DeviceObservations(t.Context(), 456, 1700000000, 1700000300)
			if err != nil && observations != nil {
				t.Fatal("failed response returned partial device observations")
			}
			if len(observations) > maxDeviceObs {
				t.Fatal("accepted oversized device observation collection")
			}
			for _, observation := range observations {
				if observation.Epoch < 1700000000 || observation.Epoch > 1700000300 {
					t.Fatal("accepted device observation outside requested range")
				}
			}
			result = observations
		case 3:
			var forecast *Forecast
			forecast, err = client.BetterForecast(t.Context(), 123)
			if err != nil && forecast != nil {
				t.Fatal("failed response returned partial forecast")
			}
			if err == nil && (forecast == nil || len(forecast.Forecast.Daily) > maxDailyForecasts || len(forecast.Forecast.Hourly) > maxHourlyForecasts) {
				t.Fatal("accepted oversized forecast collection")
			}
			result = forecast
		}
		if err != nil {
			if !errors.Is(err, ErrMalformedResponse) && !errors.Is(err, ErrNoObservation) {
				t.Fatalf("unclassified decode error: %v", err)
			}
			if !json.Valid(body) && !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("invalid JSON did not return ErrMalformedResponse: %v", err)
			}
			if len(err.Error()) > 512 || strings.Contains(err.Error(), "fuzz-private") {
				t.Fatal("decode error exceeded its size bound or exposed private input")
			}
			return
		}
		if !json.Valid(body) {
			t.Fatal("accepted invalid JSON")
		}
		if _, err := json.Marshal(result); err != nil {
			t.Fatalf("accepted response cannot be represented as JSON: %v", err)
		}
	})
}
