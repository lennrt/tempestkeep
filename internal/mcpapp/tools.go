package mcpapp

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- tool input/output types ------------------------------------------------

// NoArgs is the input type for tools that take no arguments.
type NoArgs struct{}

// ConditionsOut is the shape returned by current_conditions.
type ConditionsOut struct {
	Source              string   `json:"source"` // "live" or "archive"
	Time                string   `json:"time"`
	AgeSeconds          int64    `json:"age_seconds"`
	Station             string   `json:"station,omitempty"`
	TempF               *float64 `json:"temp_f,omitempty"`
	TempC               *float64 `json:"temp_c,omitempty"`
	FeelsLikeF          *float64 `json:"feels_like_f,omitempty"`
	DewPointF           *float64 `json:"dew_point_f,omitempty"`
	HumidityPct         *float64 `json:"humidity_pct,omitempty"`
	PressureInHg        *float64 `json:"pressure_inhg,omitempty"`
	WindMph             *float64 `json:"wind_mph,omitempty"`
	GustMph             *float64 `json:"gust_mph,omitempty"`
	WindDir             string   `json:"wind_dir,omitempty"`
	UV                  *float64 `json:"uv,omitempty"`
	SolarWm2            *float64 `json:"solar_wm2,omitempty"`
	RainTodayIn         *float64 `json:"rain_today_in,omitempty"`
	LightningStrikes1hr *int     `json:"lightning_strikes_1hr,omitempty"`
	LightningLastMi     *float64 `json:"lightning_last_distance_mi,omitempty"`
	PressureTrend       string   `json:"pressure_trend,omitempty"` // rising/falling/steady over the last 3h (archive-derived)
	PressureTrend3hInHg *float64 `json:"pressure_trend_3h_inhg,omitempty"`
	Note                string   `json:"note,omitempty"`
}

// StationSummary is one station in list_stations output.
type StationSummary struct {
	StationID int          `json:"station_id"`
	Name      string       `json:"name"`
	Latitude  float64      `json:"latitude"`
	Longitude float64      `json:"longitude"`
	Timezone  string       `json:"timezone"`
	Devices   []api.Device `json:"devices"`
}

// StationsOut is the list_stations output.
type StationsOut struct {
	Stations []StationSummary `json:"stations"`
}

// StationInfoOut is the station_info output.
type StationInfoOut struct {
	Name         string   `json:"name,omitempty"`
	StationID    int      `json:"station_id,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	Timezone     string   `json:"timezone,omitempty"`
	ElevationM   *float64 `json:"elevation_m,omitempty"`
	Observations int64    `json:"observations"`
	FirstObs     string   `json:"first_obs,omitempty"`
	LastObs      string   `json:"last_obs,omitempty"`
}

// DailySummaryArgs is the daily_summary input.
type DailySummaryArgs struct {
	Days  int    `json:"days,omitempty" jsonschema:"most-recent days to include (default 7, max 366); ignored when start is set"`
	Start string `json:"start,omitempty" jsonschema:"start date YYYY-MM-DD in local time; overrides days"`
	End   string `json:"end,omitempty" jsonschema:"end date YYYY-MM-DD in local time, inclusive; defaults to today"`
}

// DailySummaryOut is the daily_summary output.
type DailySummaryOut struct {
	From string          `json:"from"`
	To   string          `json:"to"`
	Days []store.DayStat `json:"days"`
}

// ---- tool annotations ---------------------------------------------------------

// The annotation helpers below encode the server's safety model in the hints
// the MCP spec defines, so clients can present tools appropriately:
// every query tool is read-only, and the archive write tools are append-only
// (idempotent, never destructive).

// readOnlyLive annotates a read-only tool that (also) reaches the Tempest API.
func readOnlyLive() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(true)}
}

// appendOnlyWrite annotates the archive write tools: they mutate the archive,
// but only by appending observations fetched from the API, idempotent
// (INSERT OR IGNORE) and never destructive.
func appendOnlyWrite() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		IdempotentHint:  true,
		DestructiveHint: new(false),
		OpenWorldHint:   new(true),
	}
}

// ---- tool registration ------------------------------------------------------

func registerTools(srv *mcp.Server, live *liveSource, st *store.Store) {
	// current_conditions: available whenever we have a live source or an archive.
	if live != nil || st != nil {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "current_conditions",
			Title:       "Current conditions",
			Description: "Return the latest conditions. Live data is preferred when a token is configured. An archive supplies fallback data and a 3-hour pressure trend.",
			Annotations: conditionsAnnotations(live),
		}, func(ctx context.Context, _ *mcp.CallToolRequest, _ NoArgs) (*mcp.CallToolResult, ConditionsOut, error) {
			// Prefer live; on any live failure fall back to the archive if we
			// have one, otherwise surface the error.
			if live != nil {
				if s, err := live.resolveStation(ctx); err == nil {
					if o, err := live.client.LatestStationObs(ctx, s.StationID); err == nil {
						out := liveConditions(s, o)
						if st != nil {
							// The barometric tendency needs history the live API
							// doesn't return; derive it from the archive when present.
							fillPressureTrend(ctx, st, &out)
						}
						return nil, out, nil
					} else if st == nil {
						return nil, ConditionsOut{}, err
					}
				} else if st == nil {
					return nil, ConditionsOut{}, err
				}
			}
			if st == nil {
				return nil, ConditionsOut{}, fmt.Errorf("no data source available")
			}
			latest, err := st.Latest(ctx)
			if err != nil {
				return nil, ConditionsOut{}, err
			}
			if latest == nil {
				return nil, ConditionsOut{}, fmt.Errorf("archive has no observations yet; run the collector first")
			}
			out := archiveConditions(latest)
			fillRainToday(ctx, st, &out)
			fillPressureTrend(ctx, st, &out)
			return nil, out, nil
		})
	}

	// Live tools: need the REST API (a token).
	if live != nil {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "list_stations",
			Title:       "List stations",
			Description: "List stations and devices available to the configured token. Results include location and timezone.",
			Annotations: readOnlyLive(),
		}, func(ctx context.Context, _ *mcp.CallToolRequest, _ NoArgs) (*mcp.CallToolResult, StationsOut, error) {
			stations, err := live.client.Stations(ctx)
			if err != nil {
				return nil, StationsOut{}, err
			}
			out := StationsOut{Stations: make([]StationSummary, 0, len(stations))}
			for _, s := range stations {
				out.Stations = append(out.Stations, StationSummary{
					StationID: s.StationID, Name: s.Name, Latitude: s.Latitude,
					Longitude: s.Longitude, Timezone: s.Timezone, Devices: s.Devices,
				})
			}
			return nil, out, nil
		})

		mcp.AddTool(srv, &mcp.Tool{
			Name:        "forecast",
			Title:       "Forecast",
			Description: "Return current, hourly, and daily forecast data. Temperature uses °F and wind uses mph. hours cannot exceed 240. days cannot exceed 10.",
			Annotations: readOnlyLive(),
		}, func(ctx context.Context, _ *mcp.CallToolRequest, args ForecastArgs) (*mcp.CallToolResult, ForecastOut, error) {
			hours, err := optionalBoundedInt(args.Hours, 24, 240, "hours")
			if err != nil {
				return nil, ForecastOut{}, err
			}
			days, err := optionalBoundedInt(args.Days, 10, 10, "days")
			if err != nil {
				return nil, ForecastOut{}, err
			}
			if args.StationID < 0 {
				return nil, ForecastOut{}, fmt.Errorf("station_id must be positive")
			}
			s, err := live.resolveStation(ctx)
			if err != nil {
				return nil, ForecastOut{}, err
			}
			sid := args.StationID
			if sid == 0 {
				sid = s.StationID
			}
			f, err := live.client.BetterForecast(ctx, sid)
			if err != nil {
				return nil, ForecastOut{}, err
			}
			return nil, buildForecast(s, f, hours, days), nil
		})

		mcp.AddTool(srv, &mcp.Tool{
			Name:        "station_details",
			Title:       "Station details",
			Description: "Return station metadata, coordinates, elevation in meters, timezone, and connected devices.",
			Annotations: readOnlyLive(),
		}, func(ctx context.Context, _ *mcp.CallToolRequest, args StationDetailsArgs) (*mcp.CallToolResult, StationDetailsOut, error) {
			if args.StationID < 0 {
				return nil, StationDetailsOut{}, fmt.Errorf("station_id must be positive")
			}
			sid := args.StationID
			if sid == 0 {
				s, err := live.resolveStation(ctx)
				if err != nil {
					return nil, StationDetailsOut{}, err
				}
				sid = s.StationID
			}
			stations, err := live.client.Stations(ctx)
			if err != nil {
				return nil, StationDetailsOut{}, err
			}
			var found *api.Station
			for i := range stations {
				if stations[i].StationID == sid {
					found = &stations[i]
					break
				}
			}
			if found == nil {
				return nil, StationDetailsOut{}, fmt.Errorf("requested station not found for this token")
			}
			out := StationDetailsOut{
				StationID: found.StationID, Name: found.Name,
				Latitude: found.Latitude, Longitude: found.Longitude,
				ElevationM: found.StationMeta.Elevation, Timezone: found.Timezone,
			}
			for _, d := range found.Devices {
				out.Devices = append(out.Devices, DeviceOut{DeviceID: d.DeviceID, Type: d.DeviceType, Serial: d.SerialNumber})
			}
			return nil, out, nil
		})
	}

	// Archive-backed history tools.
	if st != nil {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "station_info",
			Title:       "Station & archive info",
			Description: "Return archive coverage. Include station metadata when live access is configured.",
			Annotations: conditionsAnnotations(live),
		}, func(ctx context.Context, _ *mcp.CallToolRequest, _ NoArgs) (*mcp.CallToolResult, StationInfoOut, error) {
			cov, err := st.Coverage(ctx)
			if err != nil {
				return nil, StationInfoOut{}, err
			}
			out := StationInfoOut{Observations: cov.Count}
			if cov.MinEpoch.Valid {
				out.FirstObs = localDate(cov.MinEpoch.Int64)
			}
			if cov.MaxEpoch.Valid {
				out.LastObs = localTimeStr(cov.MaxEpoch.Int64)
			}
			// Enrich with station identity when a token is configured; best-effort.
			if live != nil {
				if s, err := live.resolveStation(ctx); err == nil {
					out.Name = s.Name
					out.StationID = s.StationID
					out.Latitude = new(s.Latitude)
					out.Longitude = new(s.Longitude)
					out.Timezone = s.Timezone
					out.ElevationM = new(s.StationMeta.Elevation)
				}
			}
			return nil, out, nil
		})

		mcp.AddTool(srv, &mcp.Tool{
			Name:        "daily_summary",
			Title:       "Daily summary",
			Description: "Return daily temperature in °F, rain in inches, and gusts in mph. days defaults to 7 and cannot exceed 366.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
		}, func(ctx context.Context, _ *mcp.CallToolRequest, args DailySummaryArgs) (*mcp.CallToolResult, DailySummaryOut, error) {
			start, end, err := resolveRange(args)
			if err != nil {
				return nil, DailySummaryOut{}, err
			}
			days, err := st.DailySummary(ctx, start, end)
			if err != nil {
				return nil, DailySummaryOut{}, err
			}
			return nil, DailySummaryOut{From: localDate(start), To: localDate(end), Days: days}, nil
		})

		mcp.AddTool(srv, &mcp.Tool{
			Name:        "records",
			Title:       "All-time records",
			Description: "Return archive-wide temperature, wind, pressure, solar, UV, rain, and lightning records.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
		}, func(ctx context.Context, _ *mcp.CallToolRequest, _ NoArgs) (*mcp.CallToolResult, RecordsOut, error) {
			rec, err := st.Records(ctx)
			if err != nil {
				return nil, RecordsOut{}, err
			}
			out := RecordsOut{Records: rec}
			if rec.HottestEpoch != nil {
				out.HottestTime = localTimeStr(*rec.HottestEpoch)
			}
			if rec.ColdestEpoch != nil {
				out.ColdestTime = localTimeStr(*rec.ColdestEpoch)
			}
			if rec.PeakGustEpoch != nil {
				out.PeakGustTime = localTimeStr(*rec.PeakGustEpoch)
			}
			if rec.LowestPressureEpoch != nil {
				out.LowestPressureTime = localTimeStr(*rec.LowestPressureEpoch)
			}
			if rec.PeakSolarEpoch != nil {
				out.PeakSolarTime = localTimeStr(*rec.PeakSolarEpoch)
			}
			if rec.PeakUVEpoch != nil {
				out.PeakUVTime = localTimeStr(*rec.PeakUVEpoch)
			}
			return nil, out, nil
		})

		registerHistoryTools(srv, st)
		registerArchiveStatusTool(srv, st)
		registerResources(srv, st)
	}
}

// RecordsOut wraps store.Records with human-readable local timestamps, so an
// agent never has to convert raw epochs itself.
type RecordsOut struct {
	store.Records
	HottestTime        string `json:"hottest_time,omitempty"`
	ColdestTime        string `json:"coldest_time,omitempty"`
	PeakGustTime       string `json:"peak_gust_time,omitempty"`
	LowestPressureTime string `json:"lowest_pressure_time,omitempty"`
	PeakSolarTime      string `json:"peak_solar_time,omitempty"`
	PeakUVTime         string `json:"peak_uv_time,omitempty"`
}

// conditionsAnnotations picks the read-only annotation for tools that consult
// the live API when a token is configured but fall back to the archive.
func conditionsAnnotations(live *liveSource) *mcp.ToolAnnotations {
	if live != nil {
		return readOnlyLive()
	}
	return &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)}
}

// fillRainToday adds today's rainfall to an archive-sourced conditions reply
// (the live API reports it directly; the archive needs a day aggregate).
// A failure leaves the optional field empty.
func fillRainToday(ctx context.Context, st *store.Store, c *ConditionsOut) {
	now := time.Now()
	days, err := st.DailySummary(ctx, localMidnight(now).Unix(), now.Unix())
	if err != nil || len(days) == 0 {
		return
	}
	// DailySummary skips days with no observations, so the last row is the most
	// recent day that has data, not necessarily today. On a stale archive that
	// would pass off an old day's rain as today's; only report a true today total.
	last := days[len(days)-1]
	if last.Day != localDate(now.Unix()) {
		return
	}
	rain := last.RainIn
	c.RainTodayIn = &rain
}

// fillPressureTrend adds the 3-hour barometric tendency from the archive, so
// current_conditions carries the short-term storm signal even when the current
// reading came from the live API. It stays silent when the archive lacks the
// history for a real trend.
func fillPressureTrend(ctx context.Context, st *store.Store, c *ConditionsOut) {
	t, ok, err := st.PressureTendency(ctx, 3*3600)
	if err != nil || !ok {
		return
	}
	c.PressureTrend = t.Category
	change := t.ChangeInHg
	c.PressureTrend3hInHg = &change
}

// ---- conditions builders ----------------------------------------------------

func liveConditions(station *api.Station, o *api.StationObs) ConditionsOut {
	c := ConditionsOut{
		Source:     "live",
		Time:       localTimeStr(o.Timestamp),
		AgeSeconds: time.Now().Unix() - o.Timestamp,
		Station:    station.Name,
	}
	if o.AirTemperature != nil {
		c.TempC = o.AirTemperature
		c.TempF = new(model.CToF(*o.AirTemperature))
	}
	if o.FeelsLike != nil {
		c.FeelsLikeF = new(model.CToF(*o.FeelsLike))
	}
	if o.DewPoint != nil {
		c.DewPointF = new(model.CToF(*o.DewPoint))
	}
	if o.RelativeHumidity != nil {
		c.HumidityPct = o.RelativeHumidity
	}
	if p := firstNonNil(o.SeaLevelPressure, o.StationPressure); p != nil {
		c.PressureInHg = new(model.MbToInHg(*p))
	}
	if o.WindAvg != nil {
		c.WindMph = new(model.MpsToMph(*o.WindAvg))
	}
	if o.WindGust != nil {
		c.GustMph = new(model.MpsToMph(*o.WindGust))
	}
	if o.WindDirection != nil {
		c.WindDir = model.Compass(*o.WindDirection)
	}
	if o.UV != nil {
		c.UV = o.UV
	}
	if o.SolarRadiation != nil {
		c.SolarWm2 = o.SolarRadiation
	}
	if o.PrecipAccumLocalDay != nil {
		c.RainTodayIn = new(model.MmToInch(*o.PrecipAccumLocalDay))
	}
	if o.LightningLast1hr != nil {
		c.LightningStrikes1hr = o.LightningLast1hr
	}
	if o.LightningLastDist != nil {
		c.LightningLastMi = new(model.KmToMile(*o.LightningLastDist))
	}
	return c
}

func archiveConditions(o *model.Obs) ConditionsOut {
	c := ConditionsOut{
		Source:     "archive",
		Time:       localTimeStr(o.Epoch),
		AgeSeconds: time.Now().Unix() - o.Epoch,
		Note:       "from local archive; feels-like/dew point derived, pressure is station (not sea-level)",
	}
	if o.AirTempC != nil {
		tF := model.CToF(*o.AirTempC)
		c.TempC = o.AirTempC
		c.TempF = new(tF)
		if o.HumidityPct != nil {
			if dp := model.DewPointC(*o.AirTempC, *o.HumidityPct); !math.IsNaN(dp) {
				c.DewPointF = new(model.CToF(dp))
			}
		}
		// Apparent temperature needs humidity in the heat and wind in the cold;
		// in the mild band it equals the air temperature. Only derive it when the
		// band's input is present, so we never report a "feels like" built on a
		// zero we don't actually have.
		switch {
		case tF >= 80 && o.HumidityPct != nil:
			c.FeelsLikeF = new(model.ApparentTempF(tF, *o.HumidityPct, 0))
		case tF <= 50 && o.WindAvgMps != nil:
			c.FeelsLikeF = new(model.ApparentTempF(tF, 0, model.MpsToMph(*o.WindAvgMps)))
		case tF > 50 && tF < 80:
			c.FeelsLikeF = new(tF)
		}
	}
	if o.HumidityPct != nil {
		c.HumidityPct = o.HumidityPct
	}
	if o.PressureMb != nil {
		c.PressureInHg = new(model.MbToInHg(*o.PressureMb))
	}
	if o.WindAvgMps != nil {
		c.WindMph = new(model.MpsToMph(*o.WindAvgMps))
	}
	if o.WindGustMps != nil {
		c.GustMph = new(model.MpsToMph(*o.WindGustMps))
	}
	if o.WindDirDeg != nil {
		c.WindDir = model.Compass(*o.WindDirDeg)
	}
	if o.UV != nil {
		c.UV = o.UV
	}
	if o.SolarWm2 != nil {
		c.SolarWm2 = o.SolarWm2
	}
	return c
}

// ---- helpers ----------------------------------------------------------------

func resolveRange(a DailySummaryArgs) (int64, int64, error) {
	if a.Days < 0 || a.Days > 366 {
		return 0, 0, fmt.Errorf("days must be in 0..366")
	}
	now := time.Now()
	// Explicit start wins: honor start..end (end defaults to today), whole end day.
	if a.Start != "" {
		start, err := parseLocalDate(a.Start)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid start: %w", err)
		}
		endDay := now
		if a.End != "" {
			endDay, err = parseLocalDate(a.End)
			if err != nil {
				return 0, 0, fmt.Errorf("invalid end: %w", err)
			}
		}
		// Include the whole end day.
		end := endDay.AddDate(0, 0, 1).Add(-time.Second)
		if start.Unix() > end.Unix() {
			return 0, 0, fmt.Errorf("start date must not be after end date")
		}
		return start.Unix(), end.Unix(), nil
	}

	// No start: a window of `days` whole calendar days. It ends on `end` when
	// given (its whole day), otherwise today. Anchoring the start to local
	// midnight keeps `days` counting calendar days rather than a rolling
	// timestamp window, which would spill a partial extra day; and honoring a
	// bare `end` stops it being silently ignored (the schema advertises it).
	days := a.Days
	if days == 0 {
		days = 7
	}
	endDay := now
	if a.End != "" {
		var err error
		endDay, err = parseLocalDate(a.End)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid end: %w", err)
		}
	}
	startMidnight := localMidnight(endDay).AddDate(0, 0, -(days - 1))
	end := now
	if a.End != "" {
		end = localMidnight(endDay).AddDate(0, 0, 1).Add(-time.Second)
	}
	return startMidnight.Unix(), end.Unix(), nil
}

// localMidnight returns 00:00 local time on t's calendar day.
func localMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func parseLocalDate(s string) (time.Time, error) {
	if len(s) != len("2006-01-02") {
		return time.Time{}, fmt.Errorf("date must be YYYY-MM-DD")
	}
	value, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("date must be YYYY-MM-DD")
	}
	return value, nil
}

func localDate(epoch int64) string    { return time.Unix(epoch, 0).Local().Format("2006-01-02") }
func localTimeStr(epoch int64) string { return time.Unix(epoch, 0).Local().Format(time.RFC3339) }

//go:fix inline
func firstNonNil(vals ...*float64) *float64 {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}
