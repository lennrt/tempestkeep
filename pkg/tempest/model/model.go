// Package model holds the Tempest data types shared across TempestKeep's Go
// tools, plus small display helpers: SI->US unit conversions, a dew-point
// calculation, and compass formatting.
//
// Everything the Tempest broadcasts and the collector stores is SI. Conversions
// here are for presentation only; the archive always keeps the raw SI values.
package model

import (
	"errors"
	"fmt"
	"math"
)

const (
	// DeviceObsFields is the exact width of a Tempest obs_st row.
	DeviceObsFields = 18
	// MaxEpochSeconds is 9999-12-31T23:59:59Z.
	MaxEpochSeconds = 253402300799
)

// ErrInvalidObservation reports malformed or physically impossible input.
var ErrInvalidObservation = errors.New("invalid observation")

// Obs is one archived Tempest observation: a row of the obs_st table written by
// the collector. Nullable sensor fields use pointers so an absent reading is
// distinct from a genuine zero.
type Obs struct {
	Epoch          int64    `json:"epoch"`
	WindLullMps    *float64 `json:"wind_lull_mps,omitempty"`
	WindAvgMps     *float64 `json:"wind_avg_mps,omitempty"`
	WindGustMps    *float64 `json:"wind_gust_mps,omitempty"`
	WindDirDeg     *float64 `json:"wind_dir_deg,omitempty"`
	PressureMb     *float64 `json:"pressure_mb,omitempty"`
	AirTempC       *float64 `json:"air_temp_c,omitempty"`
	HumidityPct    *float64 `json:"humidity_pct,omitempty"`
	IlluminanceLux *float64 `json:"illuminance_lux,omitempty"`
	UV             *float64 `json:"uv,omitempty"`
	SolarWm2       *float64 `json:"solar_wm2,omitempty"`
	RainMm         *float64 `json:"rain_mm,omitempty"`
	StrikeDistKm   *float64 `json:"strike_dist_km,omitempty"`
	StrikeCount    *float64 `json:"strike_count,omitempty"`
	BatteryV       *float64 `json:"battery_v,omitempty"`
}

// DeviceObs is one raw observation from the observations/device endpoint: the
// obs_st array WeatherFlow returns for a Tempest ("ST") device, decoded into
// named fields. It is the full row the archive stores: a superset of Obs, which
// is the presentation-focused subset the read path returns (DeviceObs adds
// wind_interval, precip_type, and report_interval_min). Nullable sensor fields
// use pointers so an absent reading is distinct from a real zero.
//
// Array layout (index -> field), per the WeatherFlow Tempest obs_st format:
//
//	0 epoch           6 pressure_mb      12 rain_mm       (mm over the report interval)
//	1 wind_lull       7 air_temp_c       13 precip_type   (0 none, 1 rain, 2 hail, 3 rain+hail)
//	2 wind_avg        8 humidity         14 strike_dist_km
//	3 wind_gust       9 illuminance_lux  15 strike_count
//	4 wind_dir       10 uv               16 battery_v
//	5 wind_interval  11 solar_wm2        17 report_interval_min
type DeviceObs struct {
	Epoch             int64
	WindLull          *float64
	WindAvg           *float64
	WindGust          *float64
	WindDir           *float64
	WindInterval      *float64
	PressureMb        *float64
	AirTempC          *float64
	Humidity          *float64
	IlluminanceLux    *float64
	UV                *float64
	SolarWm2          *float64
	RainMm            *float64
	PrecipType        *float64
	StrikeDistKm      *float64
	StrikeCount       *float64
	BatteryV          *float64
	ReportIntervalMin *float64
}

// DeviceObsFromRow validates and copies one Tempest obs_st row. JSON nulls and
// missing trailing sensor values are retained as absent values. Unknown trailing
// fields fail closed so a wire-format change cannot silently corrupt the archive.
func DeviceObsFromRow(row []*float64) (DeviceObs, error) {
	if len(row) == 0 || len(row) > DeviceObsFields || row[0] == nil {
		return DeviceObs{}, fmt.Errorf("%w: obs_st row must contain 1..%d fields and a timestamp", ErrInvalidObservation, DeviceObsFields)
	}
	if math.IsNaN(*row[0]) || math.IsInf(*row[0], 0) || math.Trunc(*row[0]) != *row[0] || *row[0] <= 0 || *row[0] > MaxEpochSeconds {
		return DeviceObs{}, fmt.Errorf("%w: timestamp must be an integral epoch in 1..%d", ErrInvalidObservation, MaxEpochSeconds)
	}
	at := func(i int) *float64 {
		if i < len(row) {
			return cloneFloat(row[i])
		}
		return nil
	}
	obs := DeviceObs{
		Epoch:             int64(*row[0]),
		WindLull:          at(1),
		WindAvg:           at(2),
		WindGust:          at(3),
		WindDir:           at(4),
		WindInterval:      at(5),
		PressureMb:        at(6),
		AirTempC:          at(7),
		Humidity:          at(8),
		IlluminanceLux:    at(9),
		UV:                at(10),
		SolarWm2:          at(11),
		RainMm:            at(12),
		PrecipType:        at(13),
		StrikeDistKm:      at(14),
		StrikeCount:       at(15),
		BatteryV:          at(16),
		ReportIntervalMin: at(17),
	}
	if err := obs.Validate(); err != nil {
		return DeviceObs{}, err
	}
	return obs, nil
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return new(*value)
}

// Validate checks an observation before it crosses a persistence boundary.
func (o DeviceObs) Validate() error {
	if o.Epoch <= 0 || o.Epoch > MaxEpochSeconds {
		return fmt.Errorf("%w: epoch must be in 1..%d", ErrInvalidObservation, MaxEpochSeconds)
	}
	checks := []struct {
		name      string
		value     *float64
		minimum   float64
		maximum   float64
		mustBeInt bool
	}{
		{"wind_lull", o.WindLull, 0, 200, false},
		{"wind_avg", o.WindAvg, 0, 200, false},
		{"wind_gust", o.WindGust, 0, 200, false},
		{"wind_direction", o.WindDir, 0, 360, false},
		{"wind_interval", o.WindInterval, 0, 3600, false},
		{"pressure", o.PressureMb, 0, 2000, false},
		{"air_temperature", o.AirTempC, -150, 150, false},
		{"relative_humidity", o.Humidity, 0, 100, false},
		{"illuminance", o.IlluminanceLux, 0, 10_000_000, false},
		{"uv", o.UV, 0, 100, false},
		{"solar_radiation", o.SolarWm2, 0, 5000, false},
		{"rain", o.RainMm, 0, 10_000, false},
		{"precipitation_type", o.PrecipType, 0, 3, true},
		{"strike_distance", o.StrikeDistKm, 0, 1000, false},
		{"strike_count", o.StrikeCount, 0, 1_000_000, true},
		{"battery", o.BatteryV, 0, 20, false},
		{"report_interval", o.ReportIntervalMin, 0, 1440, false},
	}
	for _, check := range checks {
		if err := validateReading(check.name, check.value, check.minimum, check.maximum, check.mustBeInt); err != nil {
			return err
		}
	}
	return nil
}

func validateReading(name string, value *float64, minimum, maximum float64, mustBeInt bool) error {
	if value == nil {
		return nil
	}
	if math.IsNaN(*value) || math.IsInf(*value, 0) || *value < minimum || *value > maximum || mustBeInt && math.Trunc(*value) != *value {
		return fmt.Errorf("%w: %s is outside its accepted range", ErrInvalidObservation, name)
	}
	return nil
}

// CToF converts degrees Celsius to degrees Fahrenheit.
func CToF(c float64) float64 { return c*9/5 + 32 }

// MpsToMph converts meters per second to miles per hour.
func MpsToMph(mps float64) float64 { return mps * 2.2369362920544 }

// MbToInHg converts millibars (hectopascals) to inches of mercury.
func MbToInHg(mb float64) float64 { return mb * 0.02952998057228486 }

// MmToInch converts millimeters to inches.
func MmToInch(mm float64) float64 { return mm / 25.4 }

// KmToMile converts kilometers to miles.
func KmToMile(km float64) float64 { return km / 1.609344 }

var compass16 = [16]string{
	"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE",
	"S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW",
}

// Compass returns the 16-point compass label for a wind bearing in degrees
// (the direction the wind blows FROM, matching WeatherFlow's wind_direction).
func Compass(deg float64) string {
	if math.IsNaN(deg) || math.IsInf(deg, 0) {
		return ""
	}
	d := math.Mod(deg, 360)
	if d < 0 {
		d += 360
	}
	return compass16[int(d/22.5+0.5)%16]
}

// DewPointC returns the dew point in °C from temperature (°C) and relative
// humidity (%), via the Magnus-Tetens approximation. Returns NaN for
// non-positive humidity.
func DewPointC(tC, rh float64) float64 {
	if rh <= 0 {
		return math.NaN()
	}
	const a, b = 17.62, 243.12
	g := math.Log(rh/100) + a*tC/(b+tC)
	return b * g / (a - g)
}

// HeatIndexF returns the NWS heat index ("feels like" in the heat) in °F from
// temperature (°F) and relative humidity (%). The heat index is only defined for
// warm, humid air; below 80°F it returns the air temperature unchanged, matching
// the NWS convention. It uses Rothfusz's regression with the low-humidity and
// high-humidity adjustments the NWS applies.
func HeatIndexF(tF, rh float64) float64 {
	if tF < 80 || rh < 0 {
		return tF
	}
	// Steadman's simpler form; if it stays under 80°F the full regression isn't
	// warranted, so fall back to the air temperature.
	simple := 0.5 * (tF + 61 + (tF-68)*1.2 + rh*0.094)
	if (simple+tF)/2 < 80 {
		return tF
	}
	hi := -42.379 + 2.04901523*tF + 10.14333127*rh -
		0.22475541*tF*rh - 0.00683783*tF*tF - 0.05481717*rh*rh +
		0.00122874*tF*tF*rh + 0.00085282*tF*rh*rh - 0.00000199*tF*tF*rh*rh
	switch {
	case rh < 13 && tF >= 80 && tF <= 112:
		// Dry-air correction: subtract, largest near 95°F.
		hi -= ((13 - rh) / 4) * math.Sqrt((17-math.Abs(tF-95))/17)
	case rh > 85 && tF >= 80 && tF <= 87:
		// Muggy correction: add a little.
		hi += ((rh - 85) / 10) * ((87 - tF) / 5)
	}
	return hi
}

// WindChillF returns the NWS wind-chill temperature ("feels like" in the cold)
// in °F from temperature (°F) and wind speed (mph). Wind chill is only defined
// for cold air moving faster than a walk; at or above 50°F, or at 3 mph or less,
// it returns the air temperature unchanged, matching the NWS convention.
func WindChillF(tF, windMph float64) float64 {
	if tF > 50 || windMph <= 3 {
		return tF
	}
	v := math.Pow(windMph, 0.16)
	return 35.74 + 0.6215*tF - 35.75*v + 0.4275*tF*v
}

// ApparentTempF returns the "feels like" temperature in °F: the heat index in
// warm air, the wind chill in cold air, and the air temperature in the mild band
// between where neither correction applies. Inputs are °F, relative humidity (%),
// and wind speed (mph).
func ApparentTempF(tF, rh, windMph float64) float64 {
	if tF >= 80 {
		return HeatIndexF(tF, rh)
	}
	if tF <= 50 {
		return WindChillF(tF, windMph)
	}
	return tF
}
