package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func TestPressureTendencyNearestEligibleSample(t *testing.T) {
	type sample struct {
		age      int64
		pressure *float64
	}
	for _, tc := range []struct {
		name       string
		window     int64
		samples    []sample
		wantAge    int64
		wantChange float64
	}{
		{"successor reverses direction", 10800, []sample{{14400, new(1000.0)}, {10740, new(1010.0)}}, 10740, -6},
		{"predecessor closer", 10800, []sample{{10860, new(1000.0)}, {7200, new(1010.0)}}, 10860, 4},
		{"older wins tie", 10800, []sample{{14400, new(1000.0)}, {7200, new(1010.0)}}, 14400, 4},
		{"exact target", 10800, []sample{{14400, new(1000.0)}, {10800, new(1002.0)}, {10740, new(1010.0)}}, 10800, 2},
		{"short two-hour history", 10800, []sample{{7200, new(1010.0)}}, 7200, -6},
		{"exact half window", 10800, []sample{{5400, new(1010.0)}}, 5400, -6},
		{"below half window", 10800, []sample{{5399, new(1010.0)}}, 0, 0},
		{"ineligible successor retains predecessor", 10800, []sample{{14400, new(1000.0)}, {5399, new(1010.0)}}, 14400, 4},
		{"sparse six-hour history", 10800, []sample{{21600, new(1010.0)}}, 21600, -6},
		{"null predecessor skipped", 10800, []sample{{14400, new(1000.0)}, {10801, nil}, {7200, new(1010.0)}}, 14400, 4},
		{"null successor skipped", 10800, []sample{{14400, new(1000.0)}, {10799, nil}, {7200, new(1010.0)}}, 14400, 4},
		{"one-second pair", 1, []sample{{1, new(1010.0)}}, 1, -6},
		{"one-second single sample", 1, nil, 0, 0},
		{"odd window rounded up", 3, []sample{{2, new(1010.0)}}, 2, -6},
		{"odd window rejects short pair", 3, []sample{{1, new(1010.0)}}, 0, 0},
		{"zero window defaults", 0, []sample{{7200, new(1010.0)}}, 7200, -6},
		{"negative window defaults", -1, []sample{{7200, new(1010.0)}}, 7200, -6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const latest = int64(1700000000)
			rows := []model.DeviceObs{{Epoch: latest, PressureMb: new(1004.0)}}
			for _, sample := range tc.samples {
				rows = append(rows, model.DeviceObs{Epoch: latest - sample.age, PressureMb: sample.pressure})
			}
			// The most recent observation has no pressure: it must not move the
			// window away from the most recent actual pressure reading.
			rows = append(rows, model.DeviceObs{Epoch: latest + 60, AirTempC: new(20.0)})
			s := openStoreWithDeviceObs(t, rows)
			got, ok, err := s.PressureTendency(t.Context(), tc.window)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantAge == 0 {
				if ok || got != nil {
					t.Fatalf("ineligible pressure history produced a trend: %+v, %v", got, ok)
				}
				return
			}
			if !ok || got == nil {
				t.Fatal("eligible pressure pair produced no trend")
			}
			wantRate := tc.wantChange * 10800 / float64(tc.wantAge)
			if got.At != latest || math.Abs(got.WindowHours-float64(tc.wantAge)/3600) > 1e-12 ||
				math.Abs(got.ChangeInHg-model.MbToInHg(tc.wantChange)) > 1e-12 ||
				math.Abs(got.ChangeMbPer3h-wantRate) > 1e-9 ||
				math.Abs(got.CurrentInHg-model.MbToInHg(1004)) > 1e-12 {
				t.Fatalf("trend=%+v; want span=%ds, change=%vmb, rate=%vmb/3h", got, tc.wantAge, tc.wantChange, wantRate)
			}
			wantCategory := "falling rapidly"
			if tc.wantChange > 0 {
				wantCategory = "rising rapidly"
			}
			if got.Category != wantCategory {
				t.Errorf("category=%q, want %q", got.Category, wantCategory)
			}
			if _, err := json.Marshal(got); err != nil {
				t.Fatalf("trend must remain finite and JSON-safe: %v", err)
			}
		})
	}
}

func TestPressureTendencyNearEpochStart(t *testing.T) {
	for _, tc := range []struct {
		name         string
		latest, past int64
		window       int64
		want         bool
	}{
		{"negative target with enough span", 3, 1, 4, true},
		{"negative target with insufficient span", 2, 1, 3, false},
		{"too early for half window", 1, 0, 3, false},
		{"zero epoch legacy reading", 1, 0, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openStoreWith(t, []obsRow{{epoch: tc.past, pres: new(1000.0)}, {epoch: tc.latest, pres: new(1001.0)}})
			got, ok, err := s.PressureTendency(t.Context(), tc.window)
			if err != nil || ok != tc.want {
				t.Fatalf("trend=%+v, ok=%v, err=%v; want ok=%v", got, ok, err, tc.want)
			}
			if ok && (got.At != tc.latest || got.WindowHours != float64(tc.latest-tc.past)/3600) {
				t.Fatalf("epoch arithmetic changed the pair: %+v", got)
			}
		})
	}
}

func TestPressureTendencyMissingAndErrors(t *testing.T) {
	for _, rows := range [][]model.DeviceObs{nil, {{Epoch: 1700000000, AirTempC: new(20.0)}}} {
		s := openStoreWithDeviceObs(t, rows)
		if got, ok, err := s.PressureTendency(t.Context(), 10800); err != nil || ok || got != nil {
			t.Fatalf("missing pressure: got=%+v, ok=%v, err=%v", got, ok, err)
		}
	}
	s := openStoreWithDeviceObs(t, []model.DeviceObs{
		{Epoch: 1700000000, PressureMb: new(1000.0)},
		{Epoch: 1700007200, PressureMb: new(1004.0)},
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := s.PressureTendency(ctx, 10800); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled query error=%v", err)
	}
	expired, stop := context.WithDeadline(t.Context(), time.Unix(1, 0))
	defer stop()
	if _, _, err := s.PressureTendency(expired, 10800); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired query error=%v", err)
	}
	if _, _, err := s.PressureTendency(t.Context(), 86401); !errors.Is(err, store.ErrInvalidArgument) {
		t.Fatalf("oversized window error=%v", err)
	}
	if _, ok, err := s.PressureTendency(t.Context(), 10800); err != nil || !ok {
		t.Fatalf("query after cancellation: ok=%v err=%v", ok, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.PressureTendency(t.Context(), 10800); !errors.Is(err, store.ErrClosed) {
		t.Fatalf("closed store error=%v", err)
	}
}
