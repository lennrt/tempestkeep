package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type responseDecodeProbe struct{ called bool }

func (p *responseDecodeProbe) UnmarshalJSON([]byte) error {
	p.called = true
	return nil
}

func TestResponseCollectionsAreBoundedBeforeTypedDecode(t *testing.T) {
	t.Parallel()
	array := func(item string, count int) string {
		return "[" + strings.TrimSuffix(strings.Repeat(item+",", count), ",") + "]"
	}
	for _, test := range []struct {
		name, path string
		maximum    int
		body       func(int) string
	}{
		{"stations", "stations", maxStations, func(n int) string { return `{"stations":` + array(`{}`, n) + `}` }},
		{"station devices", "stations", maxDevicesPerStation, func(n int) string {
			return `{"stations":[{"devices":` + array(`{}`, n) + `}]}`
		}},
		{"station observations", "observations/station/1", maxStationObs, func(n int) string { return `{"obs":` + array(`{}`, n) + `}` }},
		{"device observations", "observations/device/1", maxDeviceObs, func(n int) string { return `{"obs":` + array(`[100]`, n) + `}` }},
		{"device row width", "observations/device/1", 22, func(n int) string { return `{"obs":[` + array(`0`, n) + `]}` }},
		{"daily forecasts", "better_forecast", maxDailyForecasts, func(n int) string {
			return `{"forecast":{"daily":` + array(`{}`, n) + `}}`
		}},
		{"hourly forecasts", "better_forecast", maxHourlyForecasts, func(n int) string {
			return `{"forecast":{"hourly":` + array(`{}`, n) + `}}`
		}},
	} {
		for _, count := range []int{test.maximum, test.maximum + 1} {
			t.Run(fmt.Sprintf("%s/%d", test.name, count), func(t *testing.T) {
				var probe responseDecodeProbe
				err := decodeResponse([]byte(test.body(count)), test.path, &probe, nil)
				if count > test.maximum {
					if !errors.Is(err, ErrMalformedResponse) || probe.called {
						t.Fatalf("over-limit response: err=%v, typed decode=%v", err, probe.called)
					}
				} else if err != nil || !probe.called {
					t.Fatalf("at-limit response: err=%v, typed decode=%v", err, probe.called)
				}
			})
		}
	}
}

func TestResponseCollectionBoundsCheckDuplicateKeys(t *testing.T) {
	t.Parallel()
	array := func(count int) string { return "[" + strings.Repeat("{},", count-1) + "{}]" }
	for _, test := range []struct {
		name, path, body string
	}{
		{"stations", "stations", `{"stations":` + array(maxStations+1) + `,"stations":[]}`},
		{"case-insensitive stations", "stations", `{"STATIONS":` + array(maxStations+1) + `,"stations":[]}`},
		{"unicode case-fold stations", "stations", `{"ſtations":` + array(maxStations+1) + `,"stations":[]}`},
		{"devices", "stations", `{"stations":[{"devices":` + array(maxDevicesPerStation+1) + `,"devices":[]}]}`},
		{"observations", "observations/station/1", `{"obs":` + array(maxStationObs+1) + `,"obs":[]}`},
		{"daily", "better_forecast", `{"forecast":{"daily":` + array(maxDailyForecasts+1) + `,"daily":[]}}`},
		{"hourly", "better_forecast", `{"forecast":{"hourly":` + array(maxHourlyForecasts+1) + `,"hourly":[]}}`},
		{"merged forecast", "better_forecast", `{"forecast":{"hourly":` + array(maxHourlyForecasts+1) + `},"forecast":{"daily":[]}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var probe responseDecodeProbe
			err := decodeResponse([]byte(test.body), test.path, &probe, nil)
			if !errors.Is(err, ErrMalformedResponse) || probe.called {
				t.Fatalf("duplicate concealed oversized collection: err=%v, typed decode=%v", err, probe.called)
			}
		})
	}
}

func TestAPIStatusFailureIsRejectedAndNotCached(t *testing.T) {
	t.Parallel()
	for _, status := range []string{
		`{"status_code":2,"status_message":"synthetic-private-detail"}`,
		`{"status_code":-1}`,
		`{"status_code":"synthetic-private-detail"}`,
		`{"status_message":"synthetic-private-detail"}`,
	} {
		t.Run(status, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if requests.Add(1) == 1 {
					writeTestJSON(w, `{"status":`+status+`,"stations":[]}`)
					return
				}
				writeTestJSON(w, strings.Replace(stationsJSON, "{", `{"status":{"status_code":0,"status_message":"SUCCESS"},`, 1))
			}))
			t.Cleanup(srv.Close)
			client := newTestClient(t, srv.URL)
			stations, err := client.Stations(t.Context())
			if !errors.Is(err, ErrMalformedResponse) || stations != nil {
				t.Fatalf("failed status returned %v, %v", stations, err)
			}
			if strings.Contains(err.Error(), "synthetic-private-detail") {
				t.Fatalf("response detail leaked: %v", err)
			}
			stations, err = client.Stations(t.Context())
			if err != nil || len(stations) != 1 {
				t.Fatalf("recovered response = %v, %v", stations, err)
			}
		})
	}
}

func TestInvalidResponseShapeIsRejectedAndNotCached(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`null`, `{}`, `{"status":{"status_code":0}}`, `[]`, `{"obs":[]}`} {
		t.Run(body, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if requests.Add(1) == 1 {
					writeTestJSON(w, body)
					return
				}
				writeTestJSON(w, stationsJSON)
			}))
			t.Cleanup(srv.Close)
			client := newTestClient(t, srv.URL)
			if _, err := client.Stations(t.Context()); !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("invalid shape = %v, want ErrMalformedResponse", err)
			}
			stations, err := client.Stations(t.Context())
			if err != nil || len(stations) != 1 {
				t.Fatalf("recovered response = %v, %v", stations, err)
			}
		})
	}
}

func TestDeviceObservationsPreservesExplicitEmptyCollection(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`{"obs":[]}`, `{"obs":null}`, `{"status":{"status_code":0},"obs":[]}`} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeTestJSON(w, body)
			}))
			t.Cleanup(srv.Close)
			observations, err := newTestClient(t, srv.URL).DeviceObservations(t.Context(), 456, 100, 200)
			if err != nil || len(observations) != 0 {
				t.Fatalf("empty observations = %v, %v", observations, err)
			}
		})
	}
}

func TestInvalidSensorResponseIsNotCached(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{"stations", "station observation", "forecast"} {
		t.Run(endpoint, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				first := requests.Add(1) == 1
				body := stationsJSON
				switch endpoint {
				case "stations":
					if first {
						body = strings.Replace(body, `"latitude": 0`, `"latitude": 100`, 1)
					}
				case "station observation":
					body = obsJSON
					if first {
						body = strings.Replace(body, `"relative_humidity": 45`, `"relative_humidity": 101`, 1)
					}
				case "forecast":
					body = forecastJSON
					if first {
						body = strings.Replace(body, `"relative_humidity": 40`, `"relative_humidity": 101`, 1)
					}
				}
				writeTestJSON(w, body)
			}))
			t.Cleanup(srv.Close)
			client := newTestClient(t, srv.URL)
			call := func() error {
				switch endpoint {
				case "stations":
					_, err := client.Stations(t.Context())
					return err
				case "station observation":
					_, err := client.LatestStationObs(t.Context(), 123)
					return err
				default:
					_, err := client.BetterForecast(t.Context(), 123)
					return err
				}
			}
			if err := call(); !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("first response = %v, want ErrMalformedResponse", err)
			}
			if err := call(); err != nil {
				t.Fatalf("recovered response: %v", err)
			}
			if err := call(); err != nil {
				t.Fatalf("cached successful response: %v", err)
			}
			if got := requests.Load(); got != 2 {
				t.Fatalf("requests = %d, want 2", got)
			}
		})
	}
}

func TestDeviceObservationsRejectsWrongResponseIdentityAndRange(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		body string
	}{
		{"null body", `null`},
		{"missing observations", `{}`},
		{"status without observations", `{"status":{"status_code":0}}`},
		{"other device", `{"device_id":457,"type":"obs_st","obs":[[100]]}`},
		{"Air layout", `{"device_id":456,"type":"obs_air","obs":[[100]]}`},
		{"Sky layout", `{"device_id":456,"type":"obs_sky","obs":[[100]]}`},
		{"unknown layout", `{"type":"synthetic-private-detail","obs":[[100]]}`},
		{"before range", `{"obs":[[99],[100]]}`},
		{"after range", `{"obs":[[100],[201]]}`},
		{"failed status", `{"status":{"status_code":2,"status_message":"synthetic-private-detail"},"obs":[]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeTestJSON(w, test.body)
			}))
			t.Cleanup(srv.Close)
			observations, err := newTestClient(t, srv.URL).DeviceObservations(t.Context(), 456, 100, 200)
			if !errors.Is(err, ErrMalformedResponse) || observations != nil {
				t.Fatalf("invalid response returned %v, %v", observations, err)
			}
			if strings.Contains(err.Error(), "synthetic-private-detail") || strings.Contains(err.Error(), "457") {
				t.Fatalf("response detail leaked: %v", err)
			}
		})
	}
}
