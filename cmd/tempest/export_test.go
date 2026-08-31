package main

import (
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

func TestObsFieldsUnitsAndGaps(t *testing.T) {
	o := model.Obs{
		Epoch:       1700000000,
		WindAvgMps:  new(float64(10)), // -> 22.3694 mph
		AirTempC:    new(float64(20)), // -> 68 °F
		RainMm:      new(25.4),        // -> 1 in
		HumidityPct: new(float64(55)),
		// WindLullMps left nil: a real sensor gap.
	}

	si := obsFields(o, false)
	if si[0].Name != "wind_lull_mps" || si[0].Val != nil {
		t.Errorf("si lull = %+v, want nil (gap)", si[0])
	}
	if si[1].Name != "wind_avg_mps" || si[1].Val == nil || *si[1].Val != 10 {
		t.Errorf("si wind_avg = %+v, want 10 (unconverted)", si[1])
	}

	us := obsFields(o, true)
	byName := map[string]*float64{}
	for _, f := range us {
		byName[f.Name] = f.Val
	}
	if v := byName["air_temp_f"]; v == nil || *v != 68 {
		t.Errorf("us air_temp_f = %v, want 68", v)
	}
	if v := byName["rain_in"]; v == nil || *v != 1 {
		t.Errorf("us rain_in = %v, want 1", v)
	}
	if v := byName["wind_lull_mph"]; v != nil {
		t.Errorf("us lull = %v, want nil (gap survives conversion)", v)
	}
}

func TestFmtNum(t *testing.T) {
	// SI keeps full precision; US rounds to 4 decimals to shed conversion noise.
	if got := fmtNum(20.123456789, false); got != "20.123456789" {
		t.Errorf("si fmtNum = %q, want full precision", got)
	}
	if got := fmtNum(68.90000001, true); got != "68.9" {
		t.Errorf("us fmtNum = %q, want 68.9", got)
	}
	if got := fmtNum(0, true); got != "0" {
		t.Errorf("us fmtNum(0) = %q, want 0", got)
	}
}

func TestExportRange(t *testing.T) {
	// A start after the end is rejected rather than silently empty.
	if _, _, err := exportRange("2024-06-02", "2024-06-01"); err == nil {
		t.Error("expected an error when start is after end")
	}
	// A whole day's end is inclusive: end epoch is the last second of the day.
	start, end, err := exportRange("2024-06-01", "2024-06-01")
	if err != nil {
		t.Fatal(err)
	}
	if end-start != 86399 {
		t.Errorf("span = %d s, want 86399 (whole day inclusive)", end-start)
	}
}
