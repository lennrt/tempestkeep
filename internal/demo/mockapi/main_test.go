package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDeviceObservationWindow(t *testing.T) {
	t.Parallel()
	minute := time.Now().Add(-10*time.Minute).Unix() / 60 * 60
	for _, test := range []struct {
		name       string
		start, end int64
		want       int
	}{
		{"aligned boundaries", minute, minute + 120, 3},
		{"partial boundaries", minute + 1, minute + 119, 1},
		{"no complete minute", minute + 1, minute + 59, 0},
		{"single observation", minute, minute, 1},
		{"before synthetic history", minute - 90*86400, minute - 89*86400, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
				fmt.Sprintf("/observations/device/4242?time_start=%d&time_end=%d", test.start, test.end), nil)
			recorder := httptest.NewRecorder()
			handleDeviceObs(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d", recorder.Code)
			}
			var response struct {
				DeviceID int          `json:"device_id"`
				Type     string       `json:"type"`
				Obs      [][]*float64 `json:"obs"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.DeviceID != deviceID || response.Type != "obs_st" || len(response.Obs) != test.want {
				t.Fatalf("response identity/count = %d, %q, %d", response.DeviceID, response.Type, len(response.Obs))
			}
			for _, row := range response.Obs {
				if len(row) != 18 || row[0] == nil {
					t.Fatalf("invalid observation row: %v", row)
				}
				epoch := int64(*row[0])
				if epoch < test.start || epoch > test.end || epoch%60 != 0 {
					t.Fatalf("epoch %d outside minute-aligned window [%d,%d]", epoch, test.start, test.end)
				}
			}
		})
	}
}

func TestDeviceObservationRejectsInvalidRange(t *testing.T) {
	t.Parallel()
	for _, query := range []string{
		"", "time_start=bad&time_end=1", "time_start=-1&time_end=1",
		"time_start=2&time_end=1", "time_start=1&time_end=432001",
		"time_start=9223372036854775806&time_end=9223372036854775807",
	} {
		recorder := httptest.NewRecorder()
		handleDeviceObs(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/observations/device/4242?"+query, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("query %q: status = %d, want 400", query, recorder.Code)
		}
	}
}
