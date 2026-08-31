package model

import (
	"encoding/json"
	"errors"
	"math"
	"slices"
	"testing"
)

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-4 }

func TestDeviceObsFromRowCopiesAndValidates(t *testing.T) {
	epoch := float64(1_700_000_000)
	temperature := 21.5
	row := make([]*float64, DeviceObsFields)
	row[0] = &epoch
	row[7] = &temperature

	observation, err := DeviceObsFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	epoch = 1
	temperature = -50
	row[7] = nil
	if observation.Epoch != 1_700_000_000 || observation.AirTempC == nil || *observation.AirTempC != 21.5 {
		t.Fatalf("observation changed with caller-owned row: %+v", observation)
	}

	tooWide := slices.Concat(row, []*float64{new(float64)})
	if _, err := DeviceObsFromRow(tooWide); !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("wide row error = %v, want ErrInvalidObservation", err)
	}
	invalidEpoch := math.NaN()
	if _, err := DeviceObsFromRow([]*float64{&invalidEpoch}); !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("NaN epoch error = %v, want ErrInvalidObservation", err)
	}
}

func FuzzDeviceObsFromRow(f *testing.F) {
	f.Add([]byte(`[1700000000,0.2,1.1,2.3,180,3,1013.2,20.5,45,1000,5,450,0,0,10,2,2.6,1]`))
	f.Add([]byte(`[null]`))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			t.Skip()
		}
		var row []*float64
		if err := json.Unmarshal(data, &row); err != nil {
			return
		}
		observation, err := DeviceObsFromRow(row)
		if err != nil {
			if !errors.Is(err, ErrInvalidObservation) {
				t.Fatalf("error = %v, want ErrInvalidObservation", err)
			}
			return
		}
		if err := observation.Validate(); err != nil {
			t.Fatalf("accepted observation does not validate: %v", err)
		}
	})
}

func TestCToF(t *testing.T) {
	cases := []struct{ c, f float64 }{
		{0, 32},
		{100, 212},
		{25, 77},
		{-40, -40}, // the crossover point
		{37, 98.6},
	}
	for _, tc := range cases {
		if got := CToF(tc.c); !almost(got, tc.f) {
			t.Errorf("CToF(%v) = %v, want %v", tc.c, got, tc.f)
		}
	}
}

func TestMpsToMph(t *testing.T) {
	if got := MpsToMph(0); got != 0 {
		t.Errorf("MpsToMph(0) = %v, want 0", got)
	}
	if got := MpsToMph(10); !almost(got, 22.369362920544) {
		t.Errorf("MpsToMph(10) = %v, want ~22.3694", got)
	}
}

func TestMbToInHg(t *testing.T) {
	if got := MbToInHg(1013.25); !almost(got, 29.921253) {
		t.Errorf("MbToInHg(1013.25) = %v, want ~29.9213", got)
	}
}

func TestMmToInch(t *testing.T) {
	if got := MmToInch(25.4); !almost(got, 1.0) {
		t.Errorf("MmToInch(25.4) = %v, want 1.0", got)
	}
}

func TestKmToMile(t *testing.T) {
	if got := KmToMile(1.609344); !almost(got, 1.0) {
		t.Errorf("KmToMile(1.609344) = %v, want 1.0", got)
	}
}

func TestCompass(t *testing.T) {
	cases := []struct {
		deg  float64
		want string
	}{
		{0, "N"},
		{360, "N"},
		{350, "N"}, // rounds up past 348.75
		{22.5, "NNE"},
		{45, "NE"},
		{90, "E"},
		{180, "S"},
		{270, "W"},
		{-90, "W"}, // negative bearings normalize
	}
	for _, tc := range cases {
		if got := Compass(tc.deg); got != tc.want {
			t.Errorf("Compass(%v) = %q, want %q", tc.deg, got, tc.want)
		}
	}
	for _, deg := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := Compass(deg); got != "" {
			t.Errorf("Compass(%v) = %q, want empty for a non-finite bearing", deg, got)
		}
	}
}

func TestDewPointC(t *testing.T) {
	// At 100% RH the dew point equals the temperature.
	if got := DewPointC(20, 100); math.Abs(got-20) > 0.2 {
		t.Errorf("DewPointC(20,100) = %v, want ~20", got)
	}
	// Known-ish value: 20°C / 50% RH -> ~9.3°C.
	if got := DewPointC(20, 50); math.Abs(got-9.26) > 0.2 {
		t.Errorf("DewPointC(20,50) = %v, want ~9.26", got)
	}
	// Non-positive humidity is undefined.
	if got := DewPointC(20, 0); !math.IsNaN(got) {
		t.Errorf("DewPointC(20,0) = %v, want NaN", got)
	}
}

func TestHeatIndexF(t *testing.T) {
	// Below 80°F, the heat index equals the air temperature.
	if got := HeatIndexF(75, 90); got != 75 {
		t.Errorf("HeatIndexF(75,90) = %v, want 75 (below the floor)", got)
	}
	// NWS chart: 90°F at 70% RH feels like about 105°F.
	if got := HeatIndexF(90, 70); math.Abs(got-105) > 2 {
		t.Errorf("HeatIndexF(90,70) = %v, want ~105", got)
	}
	// Hot and dry: the low-humidity correction pulls the index below the air
	// temperature (95°F at 10% RH feels a touch cooler than 95).
	if got := HeatIndexF(95, 10); got > 95 {
		t.Errorf("HeatIndexF(95,10) = %v, want below 95 (dry correction)", got)
	}
}

func TestWindChillF(t *testing.T) {
	// Above 50°F, or in near-calm air, wind chill equals the air temperature.
	if got := WindChillF(60, 20); got != 60 {
		t.Errorf("WindChillF(60,20) = %v, want 60 (too warm)", got)
	}
	if got := WindChillF(30, 2); got != 30 {
		t.Errorf("WindChillF(30,2) = %v, want 30 (near calm)", got)
	}
	// NWS chart: 20°F with a 20 mph wind feels like about 4°F.
	if got := WindChillF(20, 20); math.Abs(got-4) > 1.5 {
		t.Errorf("WindChillF(20,20) = %v, want ~4", got)
	}
}

func TestApparentTempF(t *testing.T) {
	// Mild band: unchanged regardless of humidity or wind.
	if got := ApparentTempF(65, 90, 20); got != 65 {
		t.Errorf("ApparentTempF(65,...) = %v, want 65 (mild band)", got)
	}
	// Hot: routes through the heat index (above the air temperature here).
	if got := ApparentTempF(90, 70, 5); got <= 90 {
		t.Errorf("ApparentTempF(90,70,5) = %v, want above 90", got)
	}
	// Cold and windy: routes through wind chill (below the air temperature).
	if got := ApparentTempF(20, 50, 20); got >= 20 {
		t.Errorf("ApparentTempF(20,50,20) = %v, want below 20", got)
	}
}
