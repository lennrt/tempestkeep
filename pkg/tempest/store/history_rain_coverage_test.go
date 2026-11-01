package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func TestHistoricalRainCoverage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rain  []*float64
		total *float64
		reads int64
	}{
		{"missing", []*float64{nil, nil}, nil, 0},
		{"dry", []*float64{new(0.0), new(0.0)}, new(0.0), 2},
		{"partial", []*float64{new(25.4), nil}, new(1.0), 1},
		{"partial dry", []*float64{new(0.0), nil}, new(0.0), 1},
		{"positive", []*float64{new(25.4), new(12.7)}, new(1.5), 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := time.Date(2024, 1, 15, 12, 0, 0, 0, time.Local).Unix()
			rows := make([]model.DeviceObs, len(tc.rain))
			for i, r := range tc.rain {
				rows[i] = model.DeviceObs{Epoch: base + int64(i)*60, RainMm: r}
			}
			s := openStoreWithDeviceObs(t, rows)
			daily, e := s.DailySummary(t.Context(), base, base+60)
			if e != nil {
				t.Fatal(e)
			}
			monthly, e := s.PeriodSummary(t.Context(), store.PeriodMonth, base, base+60)
			if e != nil {
				t.Fatal(e)
			}
			yearly, e := s.PeriodSummary(t.Context(), store.PeriodYear, base, base+60)
			if e != nil {
				t.Fatal(e)
			}
			history, e := s.ThisDay(t.Context(), 1, 15)
			if e != nil {
				t.Fatal(e)
			}
			for _, v := range []any{daily[0], monthly[0], yearly[0], history[0]} {
				b, e := json.Marshal(v)
				if e != nil {
					t.Fatal(e)
				}
				var got struct {
					Rain         *float64 `json:"rain_in"`
					Observations int64    `json:"obs"`
					Reads        int64    `json:"rain_readings"`
				}
				if e = json.Unmarshal(b, &got); e != nil {
					t.Fatal(e)
				}
				if (got.Rain == nil) != (tc.total == nil) || got.Rain != nil && !almost(*got.Rain, *tc.total) || got.Observations != 2 || got.Reads != tc.reads {
					t.Errorf("coverage=%s want total=%v reads=%d observations=2", b, tc.total, tc.reads)
				}
			}
		})
	}
}

func TestHistoricalRainNormalsDenominator(t *testing.T) {
	s := openStoreWithDeviceObs(t, []model.DeviceObs{
		{Epoch: time.Date(2023, 1, 15, 12, 0, 0, 0, time.Local).Unix(), RainMm: new(25.4), AirTempC: new(10.0)},
		{Epoch: time.Date(2024, 1, 15, 12, 0, 0, 0, time.Local).Unix(), AirTempC: new(20.0)},
		{Epoch: time.Date(2024, 2, 15, 12, 0, 0, 0, time.Local).Unix(), AirTempC: new(20.0)},
	})
	got, e := s.MonthlyNormals(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	b, e := json.Marshal(got)
	if e != nil {
		t.Fatal(e)
	}
	var fields []struct {
		Rain      *float64 `json:"avg_rain_in"`
		Rainy     *float64 `json:"avg_rainy_days"`
		RainYears int64    `json:"rain_years"`
		Years     int64    `json:"years"`
	}
	if e = json.Unmarshal(b, &fields); e != nil {
		t.Fatal(e)
	}
	if fields[0].Rain == nil || !almost(*fields[0].Rain, 1) || fields[0].RainYears != 1 || fields[0].Years != 2 || fields[0].Rainy == nil || *fields[0].Rainy != 1 || fields[1].Rain != nil || fields[1].Rainy != nil || fields[1].RainYears != 0 {
		t.Fatalf("normals=%s", b)
	}
}
