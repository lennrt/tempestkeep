package main

import (
	"fmt"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

// ---- forecast / details (parity with the third-party mcp-server-tempest) ----

// ForecastArgs is the forecast tool input.
type ForecastArgs struct {
	StationID int `json:"station_id,omitempty" jsonschema:"station id; defaults to your resolved Tempest station"`
	Hours     int `json:"hours,omitempty" jsonschema:"hourly entries to return (default 24, max 240)"`
	Days      int `json:"days,omitempty" jsonschema:"daily entries to return (default 10, max 10)"`
}

// ForecastNow is the current-conditions summary within a forecast reply.
type ForecastNow struct {
	Time        string   `json:"time"`
	Conditions  string   `json:"conditions,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	TempF       *float64 `json:"temp_f,omitempty"`
	FeelsLikeF  *float64 `json:"feels_like_f,omitempty"`
	HumidityPct *float64 `json:"humidity_pct,omitempty"`
}

// DailyOut is one day of forecast in display units.
type DailyOut struct {
	Date              string   `json:"date"`
	Conditions        string   `json:"conditions,omitempty"`
	Icon              string   `json:"icon,omitempty"`
	HighF             *float64 `json:"high_f,omitempty"`
	LowF              *float64 `json:"low_f,omitempty"`
	PrecipProbability *int     `json:"precip_probability,omitempty"`
	Sunrise           string   `json:"sunrise,omitempty"`
	Sunset            string   `json:"sunset,omitempty"`
}

// HourlyOut is one hour of forecast in display units.
type HourlyOut struct {
	Time              string   `json:"time"`
	Conditions        string   `json:"conditions,omitempty"`
	Icon              string   `json:"icon,omitempty"`
	TempF             *float64 `json:"temp_f,omitempty"`
	FeelsLikeF        *float64 `json:"feels_like_f,omitempty"`
	PrecipProbability *int     `json:"precip_probability,omitempty"`
	WindMph           *float64 `json:"wind_mph,omitempty"`
	GustMph           *float64 `json:"gust_mph,omitempty"`
	WindDir           string   `json:"wind_dir,omitempty"`
}

// ForecastOut is the forecast tool output.
type ForecastOut struct {
	Station string       `json:"station,omitempty"`
	Current *ForecastNow `json:"current,omitempty"`
	Daily   []DailyOut   `json:"daily"`
	Hourly  []HourlyOut  `json:"hourly"`
}

// StationDetailsArgs is the station_details tool input.
type StationDetailsArgs struct {
	StationID int `json:"station_id,omitempty" jsonschema:"station id; defaults to your resolved Tempest station"`
}

// DeviceOut is one device in station_details output.
type DeviceOut struct {
	DeviceID int    `json:"device_id"`
	Type     string `json:"type"`
	Serial   string `json:"serial,omitempty"`
}

// StationDetailsOut is the station_details tool output.
type StationDetailsOut struct {
	StationID  int         `json:"station_id"`
	Name       string      `json:"name"`
	Latitude   float64     `json:"latitude"`
	Longitude  float64     `json:"longitude"`
	ElevationM float64     `json:"elevation_m"`
	Timezone   string      `json:"timezone"`
	Devices    []DeviceOut `json:"devices"`
}

func buildForecast(station *api.Station, f *api.Forecast, hours, days int) ForecastOut {
	out := ForecastOut{Station: station.Name}

	cc := f.CurrentConditions
	now := &ForecastNow{Time: localTimeStr(cc.Time), Conditions: cc.Conditions, Icon: cc.Icon}
	if cc.AirTemperature != nil {
		now.TempF = new(model.CToF(*cc.AirTemperature))
	}
	if cc.FeelsLike != nil {
		now.FeelsLikeF = new(model.CToF(*cc.FeelsLike))
	}
	if cc.RelativeHumidity != nil {
		now.HumidityPct = cc.RelativeHumidity
	}
	out.Current = now

	for i, d := range f.Forecast.Daily {
		if i >= days {
			break
		}
		do := DailyOut{Date: localDate(d.DayStartLocal), Conditions: d.Conditions, Icon: d.Icon, PrecipProbability: d.PrecipProbability}
		if d.AirTempHigh != nil {
			do.HighF = new(model.CToF(*d.AirTempHigh))
		}
		if d.AirTempLow != nil {
			do.LowF = new(model.CToF(*d.AirTempLow))
		}
		if d.Sunrise > 0 {
			do.Sunrise = hhmm(d.Sunrise)
		}
		if d.Sunset > 0 {
			do.Sunset = hhmm(d.Sunset)
		}
		out.Daily = append(out.Daily, do)
	}

	for i, h := range f.Forecast.Hourly {
		if i >= hours {
			break
		}
		ho := HourlyOut{Time: localTimeStr(h.Time), Conditions: h.Conditions, Icon: h.Icon, PrecipProbability: h.PrecipProbability}
		if h.AirTemperature != nil {
			ho.TempF = new(model.CToF(*h.AirTemperature))
		}
		if h.FeelsLike != nil {
			ho.FeelsLikeF = new(model.CToF(*h.FeelsLike))
		}
		if h.WindAvg != nil {
			ho.WindMph = new(model.MpsToMph(*h.WindAvg))
		}
		if h.WindGust != nil {
			ho.GustMph = new(model.MpsToMph(*h.WindGust))
		}
		if h.WindDirection != nil {
			ho.WindDir = model.Compass(*h.WindDirection)
		}
		out.Hourly = append(out.Hourly, ho)
	}
	return out
}

func hhmm(epoch int64) string { return time.Unix(epoch, 0).Local().Format("15:04") }

func optionalBoundedInt(v, def, hi int, name string) (int, error) {
	if v == 0 {
		return def, nil
	}
	if v < 0 || v > hi {
		return 0, fmt.Errorf("%s must be in 0..%d", name, hi)
	}
	return v, nil
}

// orDefault returns v unless it is zero, in which case it returns def. It lets a
// numeric tool argument stay omittable (the JSON zero value) while still landing
// on a sensible default.
func orDefault(v, def float64) float64 {
	if v == 0 {
		return def
	}
	return v
}
