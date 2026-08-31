package main

// MCP resources and prompts: the archive's schema and a data
// dictionary, so an agent can orient itself (especially before reaching for
// query_sql), plus prompts that package the server's best workflows.

import (
	"context"
	"strings"

	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	schemaURI     = "tempest://archive/schema"
	dictionaryURI = "tempest://archive/data-dictionary"
)

// dataDictionary documents every obs_st column: meaning, unit, and the quirks
// an agent needs to aggregate correctly (per-interval rain, multi-device rows).
const dataDictionary = `# TempestKeep archive: data dictionary

The archive is a SQLite file. Table **obs_st** holds one row per observation
(1-minute resolution over REST) in the SI units the Tempest reports. The query
tools return US display units; **query_sql returns these raw SI values**.

| Column | Meaning | Unit / values |
|---|---|---|
| device_id | Tempest device that reported the row | integer id |
| epoch | observation time | unix seconds, UTC |
| wind_lull | lowest 3-second wind over the interval | m/s |
| wind_avg | average wind over the interval | m/s |
| wind_gust | highest 3-second wind over the interval | m/s |
| wind_dir | direction the wind blows FROM | degrees, 0 = N |
| wind_interval | wind sampling interval | seconds |
| pressure_mb | station (not sea-level) pressure | millibars / hPa |
| air_temp_c | air temperature | °C |
| humidity | relative humidity | % |
| illuminance_lux | ambient light | lux |
| uv | UV index | index |
| solar_wm2 | solar radiation | W/m² |
| rain_mm | rain accumulated **over this report interval** (sum rows for totals) | mm |
| precip_type | precipitation kind | 0 none, 1 rain, 2 hail, 3 rain+hail |
| strike_dist_km | avg lightning strike distance | km |
| strike_count | lightning strikes **in this interval** (sum rows for totals) | count |
| battery_v | sensor battery | volts |
| report_interval_min | reporting interval | minutes |
| source | how the row was collected | 'rest' |

Primary key: (device_id, epoch); writes are INSERT OR IGNORE, so the archive
is append-only and idempotent. NULL means the sensor had no reading.

Table **meta** (key, value) holds collector state, e.g. backfill_cursor and
backfill_complete for the resumable backward backfill.

Notes for correct queries:
- Rows from every device share the table; a typical archive has one Tempest.
  Filter by device_id if you have more than one device.
- rain_mm and strike_count are per-interval deltas: totals need SUM, not MAX.
- Daily/monthly grouping should use local time: the built-in tools
  (daily_summary, period_summary, this_day_in_history) already do this; prefer
  them over hand-written strftime queries.
`

// registerResources adds the archive resources and the prompts. st serves the
// live schema so the resource always reflects the actual file.
func registerResources(srv *mcp.Server, st *store.Store) {
	srv.AddResource(&mcp.Resource{
		URI:         schemaURI,
		Name:        "archive-schema",
		Title:       "Archive SQL schema",
		Description: "The live SQL schema (DDL) of the local observation archive: the tables and indexes query_sql can use.",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		res, err := st.Query(ctx, "SELECT sql FROM sqlite_master WHERE sql IS NOT NULL ORDER BY name", 0)
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		for _, row := range res.Rows {
			if s, ok := row[0].(string); ok {
				b.WriteString(s)
				b.WriteString(";\n\n")
			}
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI: schemaURI, MIMEType: "text/plain", Text: b.String(),
		}}}, nil
	})

	srv.AddResource(&mcp.Resource{
		URI:         dictionaryURI,
		Name:        "archive-data-dictionary",
		Title:       "Archive data dictionary",
		Description: "Column-by-column meaning and units for the observation archive, with the pitfalls to avoid when writing SQL against it.",
		MIMEType:    "text/markdown",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI: dictionaryURI, MIMEType: "text/markdown", Text: dataDictionary,
		}}}, nil
	})

	registerPrompts(srv)
}

// registerPrompts adds prompts that package the server's two best workflows:
// a data-grounded weather report and the archive build/maintain loop.
func registerPrompts(srv *mcp.Server) {
	srv.AddPrompt(&mcp.Prompt{
		Name:        "weather_report",
		Title:       "Station weather report",
		Description: "Write a personal weather report from this station's live conditions, forecast, and local history.",
		Arguments: []*mcp.PromptArgument{{
			Name:        "focus",
			Description: "optional angle to emphasize, e.g. 'rain', 'wind', 'gardening', 'running'",
		}},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		focus := req.Params.Arguments["focus"]
		text := `Write a weather report for my station using its own data, not generic knowledge:

1. current_conditions for what it's like right now.
2. pressure_trend for the short-term signal: is the barometer rising or falling?
3. forecast for what's coming (next hours + days).
4. daily_summary for the recent trend (last 7 days).
5. this_day_in_history and records to put today in context: is this unusual for here?

Keep it conversational and concrete; cite the numbers you used.`
		if focus != "" {
			text += "\n\nEmphasize: " + focus + "."
		}
		return &mcp.GetPromptResult{
			Description: "A data-grounded weather report for this station",
			Messages: []*mcp.PromptMessage{{
				Role:    "user",
				Content: &mcp.TextContent{Text: text},
			}},
		}, nil
	})

	srv.AddPrompt(&mcp.Prompt{
		Name:        "climate_review",
		Title:       "Review the station's local climate",
		Description: "Summarize what the archive reveals about this station's local climate: seasons, extremes, and trend.",
		Arguments: []*mcp.PromptArgument{{
			Name:        "focus",
			Description: "optional angle to emphasize, e.g. 'gardening', 'solar', 'storms', 'running'",
		}},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		text := `Review my station's local climate from its own archive, not generic knowledge:

1. climate_normals for the shape of the year: the twelve-month baseline.
2. temperature_trend for the direction of travel: is it warming or cooling, and how strong is the fit?
3. climate_indices for the seasonal character: frost, ice, summer, and hot days, and tropical nights.
4. comfort_stats for the human extremes: the worst heat index, the coldest wind chill, the muggiest day.
5. lightning_activity, solar_stats, and wind_stats for the storm, sun, and wind story.
6. records to anchor the all-time extremes.

Tie it together into a portrait of this place: what the seasons feel like, what stands out,
and what the trend suggests. Cite the numbers you used.`
		if focus := req.Params.Arguments["focus"]; focus != "" {
			text += "\n\nEmphasize: " + focus + "."
		}
		return &mcp.GetPromptResult{
			Description: "A data-grounded portrait of the station's local climate",
			Messages: []*mcp.PromptMessage{{
				Role:    "user",
				Content: &mcp.TextContent{Text: text},
			}},
		}, nil
	})

	srv.AddPrompt(&mcp.Prompt{
		Name:        "build_archive",
		Title:       "Build & maintain the local archive",
		Description: "Download the station's full history into the local archive and keep it current.",
	}, func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "The archive build/maintain loop",
			Messages: []*mcp.PromptMessage{{
				Role: "user",
				Content: &mcp.TextContent{Text: `Build my local weather archive and keep it current:

1. archive_status to see what's already stored (coverage, freshness, gaps).
2. If history is missing, call backfill_archive repeatedly until has_more is
   false; each call fetches one bounded batch and resumes automatically.
   Report progress as you go (rows added, coverage so far).
3. sync_archive to top up to the present.
4. Finish with archive_status and summarize: total observations, date span,
   freshness, and any remaining gaps worth backfilling (pass start/end to
   backfill_archive to target a specific gap).

Every write is idempotent (INSERT OR IGNORE), so repeating a step is always safe.`},
			}},
		}, nil
	})
}
