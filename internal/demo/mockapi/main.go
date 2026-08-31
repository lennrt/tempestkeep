// Command mockapi is a tiny stand-in for the WeatherFlow REST API, used only to
// record the README demos (docs/*.tape) deterministically: no station, token,
// or network required. Point the real binaries at it with TEMPEST_API_BASE.
//
// It synthesizes summer weather for one station: a diurnal temperature swing,
// an afternoon breeze, daylight solar and UV values, and one evening shower.
// The same model backs every endpoint, so historical and live values agree.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"
)

const (
	stationID = 42
	deviceID  = 4242
	name      = "Tempest Ridge"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8910", "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/stations", handleStations)
	mux.HandleFunc("/observations/station/", handleStationObs)
	mux.HandleFunc("/observations/device/", handleDeviceObs)
	mux.HandleFunc("/better_forecast", handleForecast)

	server := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	log.Printf("mock WeatherFlow API on http://%s (station %q)", *addr, name)
	log.Fatal(server.ListenAndServe())
}

// ---- the weather model -------------------------------------------------------

// wx is the synthetic condition of the atmosphere at one moment.
type wx struct {
	tempC, humidity, pressureMb   float64
	windAvg, windGust, windDirDeg float64
	uv, solarWm2, lux             float64
	rainMmPerMin                  float64
	strikes                       float64
}

// showerDaysAgo places the demo thundershower: that evening 18:00–19:00 local.
const showerDaysAgo = 3

// modelAt evaluates the synthetic weather at t (local time).
func modelAt(t time.Time) wx {
	h := float64(t.Hour()) + float64(t.Minute())/60

	// Diurnal temperature: 16°C (~61°F) near 05:00, 33°C (~91°F) near 15:00.
	temp := 24.5 - 8.5*math.Cos((h-3)/24*2*math.Pi)
	hum := 72 - 38*(temp-16)/17 // dry afternoons, damper nights

	// Daylight bell for solar/UV, zero outside ~06:00–20:00.
	sun := math.Sin((h - 6) / 14 * math.Pi)
	if h < 6 || h > 20 {
		sun = 0
	}

	// Afternoon westerly: calm mornings, ~4 m/s mid-afternoon.
	breeze := 0.4 + 3.6*math.Max(0, math.Sin((h-10)/10*math.Pi))

	w := wx{
		tempC:      temp,
		humidity:   hum,
		pressureMb: 1013 + 2.5*math.Sin(h/24*2*math.Pi+1),
		windAvg:    breeze,
		windGust:   breeze * 1.8,
		windDirDeg: 250 + 40*math.Sin(h/3), // W–WNW, wandering
		uv:         9 * sun,
		solarWm2:   940 * sun,
		lux:        110000 * sun,
	}

	// The shower: showerDaysAgo back, 18:00–19:00, with a few strikes.
	if daysAgo(t) == showerDaysAgo && t.Hour() == 18 {
		w.rainMmPerMin = 0.18
		if t.Minute()%12 == 0 {
			w.strikes = 1
		}
		w.windGust += 4
		w.uv, w.solarWm2, w.lux = 0, 0, 0
		w.humidity = math.Min(w.humidity+25, 98)
	}
	return w
}

func daysAgo(t time.Time) int {
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	tMid := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	return int(midnight.Sub(tMid).Hours() / 24)
}

// iconAt picks the forecast icon the same model would justify.
func iconAt(t time.Time) (icon, conditions string) {
	switch {
	case daysAgo(t) == showerDaysAgo && t.Hour() >= 17 && t.Hour() <= 19:
		return "possibly-thunderstorm-day", "Possible Thunderstorm"
	case t.Hour() < 6 || t.Hour() >= 20:
		return "clear-night", "Clear"
	case t.Hour() < 9:
		return "partly-cloudy-day", "Partly Cloudy" // morning marine layer
	default:
		return "clear-day", "Sunny"
	}
}

// ---- handlers ----------------------------------------------------------------

func handleStations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"stations": []map[string]any{{
			"station_id": stationID,
			"name":       name,
			"latitude":   0.0,
			"longitude":  0.0,
			"timezone":   "UTC",
			"station_meta": map[string]any{
				"elevation": 0.0,
			},
			"devices": []map[string]any{{
				"device_id":     deviceID,
				"device_type":   "ST",
				"serial_number": "ST-00042424",
			}},
		}},
	})
}

func handleStationObs(w http.ResponseWriter, _ *http.Request) {
	now := time.Now()
	m := modelAt(now)
	feels := m.tempC
	if m.tempC > 27 {
		feels = m.tempC + 1.2 // a touch of heat index on hot afternoons
	}
	dew := m.tempC - (100-m.humidity)/5
	writeJSON(w, map[string]any{
		"obs": []map[string]any{{
			"timestamp":              now.Unix(),
			"air_temperature":        round1(m.tempC),
			"relative_humidity":      round1(m.humidity),
			"station_pressure":       round1(m.pressureMb - 39), // ~331 m below sea-level reading
			"sea_level_pressure":     round1(m.pressureMb),
			"wind_avg":               round1(m.windAvg),
			"wind_gust":              round1(m.windGust),
			"wind_direction":         round1(m.windDirDeg),
			"uv":                     round1(m.uv),
			"solar_radiation":        round1(m.solarWm2),
			"brightness":             round1(m.lux),
			"feels_like":             round1(feels),
			"dew_point":              round1(dew),
			"precip_accum_local_day": 0.0,
		}},
	})
}

func handleDeviceObs(w http.ResponseWriter, r *http.Request) {
	start, err1 := strconv.ParseInt(r.URL.Query().Get("time_start"), 10, 64)
	end, err2 := strconv.ParseInt(r.URL.Query().Get("time_end"), 10, 64)
	if err1 != nil || err2 != nil {
		http.Error(w, "time_start/time_end required", http.StatusBadRequest)
		return
	}
	// History exists for the last 45 days; earlier windows return empty, which
	// is how a backward-walking backfill discovers that history has ended.
	oldest := time.Now().AddDate(0, 0, -45).Unix()
	if start < oldest {
		start = oldest
	}
	var obs [][]any
	for e := start - start%60; e <= end && e <= time.Now().Unix(); e += 60 {
		m := modelAt(time.Unix(e, 0))
		row := make([]any, 18)
		row[0] = e
		row[1] = round1(m.windAvg * 0.6) // lull
		row[2] = round1(m.windAvg)
		row[3] = round1(m.windGust)
		row[4] = math.Round(m.windDirDeg)
		row[5] = 3                         // wind sample interval, seconds
		row[6] = round1(m.pressureMb - 39) // station pressure at ~331 m
		row[7] = round1(m.tempC)
		row[8] = round1(m.humidity)
		row[9] = math.Round(m.lux)
		row[10] = round1(m.uv)
		row[11] = math.Round(m.solarWm2)
		row[12] = m.rainMmPerMin
		row[13] = 0 // precip type
		if m.strikes > 0 {
			row[14] = 12.0 // strike distance, km
			row[15] = m.strikes
		}
		row[16] = 2.71 // battery volts
		row[17] = 1    // report interval, minutes
		obs = append(obs, row)
	}
	writeJSON(w, map[string]any{"obs": obs})
}

func handleForecast(w http.ResponseWriter, _ *http.Request) {
	now := time.Now()
	icon, cond := iconAt(now)
	m := modelAt(now)

	daily := make([]map[string]any, 0, 5)
	for i := range 5 {
		day := now.AddDate(0, 0, i)
		mid := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local)
		hi, lo := 32.5+float64((i*3)%4), 16.0+float64((i*2)%3) // gentle day-to-day variety
		dIcon, dCond, precip := "clear-day", "Sunny", 0
		switch i {
		case 1:
			dIcon, dCond, precip = "possibly-thunderstorm-day", "Possible Thunderstorm", 40
		case 4:
			dIcon, dCond = "partly-cloudy-day", "Partly Cloudy"
		}
		daily = append(daily, map[string]any{
			"day_start_local":    mid.Unix(),
			"conditions":         dCond,
			"icon":               dIcon,
			"sunrise":            mid.Add(5*time.Hour + 48*time.Minute).Unix(),
			"sunset":             mid.Add(20*time.Hour + 2*time.Minute).Unix(),
			"air_temp_high":      hi,
			"air_temp_low":       lo,
			"precip_probability": precip,
		})
	}

	hourly := make([]map[string]any, 0, 24)
	for i := range 24 {
		t := now.Add(time.Duration(i) * time.Hour)
		hm := modelAt(t)
		hIcon, hCond := iconAt(t)
		hourly = append(hourly, map[string]any{
			"time":               t.Unix(),
			"conditions":         hCond,
			"icon":               hIcon,
			"air_temperature":    round1(hm.tempC),
			"relative_humidity":  round1(hm.humidity),
			"precip_probability": 0,
			"wind_avg":           round1(hm.windAvg),
			"wind_gust":          round1(hm.windGust),
		})
	}

	writeJSON(w, map[string]any{
		"current_conditions": map[string]any{
			"time":              now.Unix(),
			"conditions":        cond,
			"icon":              icon,
			"air_temperature":   round1(m.tempC),
			"feels_like":        round1(m.tempC),
			"relative_humidity": round1(m.humidity),
		},
		"forecast": map[string]any{
			"daily":  daily,
			"hourly": hourly,
		},
	})
}

// ---- helpers -----------------------------------------------------------------

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode: %v", err)
	}
}
