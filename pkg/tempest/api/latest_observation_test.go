package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestStationObservationTimestampOrder(t *testing.T) {
	for _, tc := range []struct {
		name, rows string
		want       float64
		malformed  bool
	}{
		{"ascending", `{"timestamp":100,"air_temperature":1},{"timestamp":200,"air_temperature":2}`, 2, false},
		{"descending", `{"timestamp":200,"air_temperature":2},{"timestamp":100,"air_temperature":1}`, 2, false},
		{"shuffled", `{"timestamp":100,"air_temperature":1},{"timestamp":300,"air_temperature":3},{"timestamp":200,"air_temperature":2}`, 3, false},
		{"tie first wins", `{"timestamp":200,"air_temperature":2},{"timestamp":200,"air_temperature":3}`, 2, false},
		{"singleton", `{"timestamp":200,"air_temperature":2}`, 2, false},
		{"malformed older", `{"timestamp":200,"air_temperature":2},{"timestamp":100,"air_temperature":999}`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"obs":[%s]}`, tc.rows)
			}))
			t.Cleanup(srv.Close)
			client := newTestClient(t, srv.URL)
			got, err := client.LatestStationObs(t.Context(), 1)
			if tc.malformed {
				if !errors.Is(err, ErrMalformedResponse) {
					t.Fatalf("error=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.AirTemperature == nil || *got.AirTemperature != tc.want {
				t.Fatalf("observation=%+v, want temperature %v", got, tc.want)
			}
		})
	}
}
