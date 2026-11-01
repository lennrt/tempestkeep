package store_test

import (
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

func TestComfortRequiresPairedInputs(t *testing.T) {
	const base int64 = 1700000100 // inside one quarter-hour bucket
	for _, tc := range []struct {
		name string
		rows []model.DeviceObs
	}{
		{"hot", []model.DeviceObs{{Epoch: base, AirTempC: new(35.0)}, {Epoch: base + 60, Humidity: new(90.0)}}},
		{"cold", []model.DeviceObs{{Epoch: base, AirTempC: new(0.0)}, {Epoch: base + 60, WindAvg: new(10.0)}}},
		{"different buckets", []model.DeviceObs{{Epoch: base, AirTempC: new(35.0)}, {Epoch: base + 900, Humidity: new(90.0)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openStoreWithDeviceObs(t, tc.rows)
			got, err := s.ComfortStatistics(t.Context(), base, base+1000)
			if err != nil {
				t.Fatal(err)
			}
			if got.HottestFeelsLikeF != nil || got.ColdestFeelsLikeF != nil || got.MuggiestDewPointF != nil {
				t.Fatalf("unpaired sensors invented comfort: %+v", got)
			}
			if got.DaysObserved != 1 {
				t.Fatalf("observed days=%d", got.DaysObserved)
			}
		})
	}
}

func TestComfortIgnoresUnpairedOutliers(t *testing.T) {
	const base int64 = 1700000100
	for _, tc := range []struct {
		name   string
		paired model.DeviceObs
		extra  []model.DeviceObs
		want   float64
		dew    bool
	}{
		{"hot", model.DeviceObs{Epoch: base, AirTempC: new(35.0), Humidity: new(50.0)}, []model.DeviceObs{{Epoch: base + 60, AirTempC: new(45.0)}, {Epoch: base + 120, Humidity: new(90.0)}}, model.ApparentTempF(95, 50, 0), true},
		{"cold", model.DeviceObs{Epoch: base, AirTempC: new(0.0), WindAvg: new(0.0)}, []model.DeviceObs{{Epoch: base + 60, AirTempC: new(-20.0)}, {Epoch: base + 120, WindAvg: new(20.0)}}, model.ApparentTempF(32, 0, 0), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := append([]model.DeviceObs{tc.paired}, tc.extra...)
			s := openStoreWithDeviceObs(t, rows)
			got, err := s.ComfortStatistics(t.Context(), base, base+120)
			if err != nil {
				t.Fatal(err)
			}
			if got.HottestFeelsLikeF == nil || !almost(*got.HottestFeelsLikeF, tc.want) || got.ColdestFeelsLikeF == nil || !almost(*got.ColdestFeelsLikeF, tc.want) {
				t.Fatalf("feels=%+v want %v", got, tc.want)
			}
			if tc.dew {
				want := model.CToF(model.DewPointC(35, 50))
				if got.MuggiestDewPointF == nil || !almost(*got.MuggiestDewPointF, want) {
					t.Fatalf("dew=%v want %v", got.MuggiestDewPointF, want)
				}
			}
		})
	}
}

func TestComfortMildAndZeroInputs(t *testing.T) {
	const base int64 = 1700000100
	for _, tc := range []struct {
		name string
		row  model.DeviceObs
		want float64
	}{
		{"mild alone", model.DeviceObs{Epoch: base, AirTempC: new(20.0)}, 68},
		{"zero humidity", model.DeviceObs{Epoch: base, AirTempC: new(35.0), Humidity: new(0.0)}, model.ApparentTempF(95, 0, 0)},
		{"zero wind", model.DeviceObs{Epoch: base, AirTempC: new(0.0), WindAvg: new(0.0)}, 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openStoreWithDeviceObs(t, []model.DeviceObs{tc.row})
			got, err := s.ComfortStatistics(t.Context(), base, base)
			if err != nil {
				t.Fatal(err)
			}
			if got.HottestFeelsLikeF == nil || !almost(*got.HottestFeelsLikeF, tc.want) {
				t.Fatalf("feels=%+v want %v", got, tc.want)
			}
		})
	}
}

func TestComfortRetainsQuarterHourPairedMeans(t *testing.T) {
	const base int64 = 1700000100
	s := openStoreWithDeviceObs(t, []model.DeviceObs{
		{Epoch: base, AirTempC: new(30.0), Humidity: new(40.0)},
		{Epoch: base + 60, AirTempC: new(40.0), Humidity: new(60.0)},
	})
	got, err := s.ComfortStatistics(t.Context(), base, base+60)
	if err != nil {
		t.Fatal(err)
	}
	want := model.ApparentTempF(95, 50, 0)
	dew := model.CToF(model.DewPointC(35, 50))
	if got.HottestFeelsLikeF == nil || !almost(*got.HottestFeelsLikeF, want) || got.MuggiestDewPointF == nil || !almost(*got.MuggiestDewPointF, dew) {
		t.Fatalf("paired means=%+v want feels %v dew %v", got, want, dew)
	}
}

func TestComfortIncludesEveryAvailableCandidate(t *testing.T) {
	const base int64 = 1700000100
	s := openStoreWithDeviceObs(t, []model.DeviceObs{
		{Epoch: base, AirTempC: new(35.0), Humidity: new(50.0)},
		{Epoch: base + 60, AirTempC: new(0.0), WindAvg: new(0.0)},
	})
	got, err := s.ComfortStatistics(t.Context(), base, base+60)
	if err != nil {
		t.Fatal(err)
	}
	hot := model.ApparentTempF(95, 50, 0)
	if got.HottestFeelsLikeF == nil || !almost(*got.HottestFeelsLikeF, hot) || got.ColdestFeelsLikeF == nil || !almost(*got.ColdestFeelsLikeF, 32) {
		t.Fatalf("candidate extrema=%+v want min32 max%v", got, hot)
	}
}
