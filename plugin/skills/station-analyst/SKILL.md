---
name: station-analyst
description: >-
  Analyze a local WeatherFlow Tempest archive through the TempestKeep MCP tools.
  Use for records, extremes, comparisons, patterns, gardening, solar, laundry,
  running, and other station-history questions.
---

Analyze data from the user's station. The `tempestkeep` MCP server provides a local
SQLite archive of one-minute observations and live tools. Map each question to the
smallest query that can answer it.

## Pick the cheapest tool that answers the question

| Question shape | Tool |
|---|---|
| Extremes across all time | `records` |
| "This date over the years" | `this_day_in_history` |
| Month-vs-month, year-vs-year | `period_summary` |
| Day-by-day inside a window | `daily_summary` |
| Wind direction/strength patterns | `wind_rose` (accepts a date range; compare seasons by calling it twice) |
| "When is it usually coldest/warmest/windiest?" (time of day) | `climatology` (the average day, hour by hour) |
| Heating/cooling energy demand, growing-season heat units | `degree_days` (HDD/CDD/GDD, configurable bases) |
| Frost/hot-day counts, tropical nights, year-over-year trend | `climate_indices` (pass yearly=true) |
| "Is a storm coming?" / barometer rising or falling | `pressure_trend` (3-hour tendency) |
| Rainfall totals, dry-spell / wet-spell length, wettest day | `rain_stats` |
| "Is this month unusual?" / the monthly baseline | `climate_normals`, then compare a `period_summary` month against it |
| Time series for a chart or trend | `get_observations` (auto-downsamples) |
| A question that no shaped tool supports | `query_sql` |

Before the first `query_sql` call, read `tempest://archive/schema` and
`tempest://archive/data-dictionary`. They define columns, units, and limits.

## Rules that keep answers correct

- **Raw tables use SI units. Shaped tools return US display units.** `obs_st`
  stores °C, m/s, mm, and millibars. When you use `query_sql`, convert output and
  name its units.
- **`epoch` is UTC seconds. Calendar questions use local days.** Prefer shaped
  tools for calendar queries. In raw SQL, account for the local UTC offset.
- **Rain (`rain_mm`) and lightning (`strike_count`) are per-minute increments:**
  SUM them over a window; never average or MAX them for totals.
- **NULL means a missing sensor value.** Use aggregates that skip NULL. Check
  `archive_status` before stating that an event never happened.
- A "rainy day" is ≥ 0.01 in; "calm" wind is < ~1.1 mph (direction is noise below
  that; `wind_rose` already excludes it).

## Worked examples

**"What was the windiest day of the year?"** → `query_sql`: group by local day,
`MAX(wind_gust)` for the peak and `AVG(wind_avg)` for sustained wind; report both,
they answer different ideas of "windiest":

```sql
SELECT date(epoch, 'unixepoch', 'localtime') AS day,
       ROUND(MAX(wind_gust) * 2.23694, 1) AS peak_gust_mph,
       ROUND(AVG(wind_avg) * 2.23694, 1)  AS sustained_mph
FROM obs_st WHERE epoch >= strftime('%s', 'now', '-1 year')
GROUP BY day ORDER BY peak_gust_mph DESC LIMIT 5;
```

**"When do my solar panels get sun?"** → `query_sql` over
`solar_wm2` grouped by hour of local day for the last 90 days; report the peak-hours
window and how it shifts by season (repeat for a winter window if the archive is
deep enough). The same shape answers "what time of day is UV highest?" (use `uv`,
and mention the UV 8+ hours for sunscreen rather than panels).

**"When can I plant tomatoes?"** → report frost risk and warmth: last
frost date (`query_sql`: latest spring day per year with `MIN(air_temp_c) <= 0`),
overnight lows the past two weeks (`daily_summary`), and the forecast for cold
snaps. Answer the gardening question the data question sits inside.

**"Has it been a dry month?"** → `period_summary` for this month across years:
rain total and rainy-day count against the same month in prior years beats a bare
number.

## Answer style

Give the answer first. Then give the supporting numbers. Use small tables for
rankings. Use sentences for other results. Write dates as "March 14, 2025". If
the archive cannot support the answer, state the missing range or coverage gap.
Offer `/tempestkeep:build-archive` when more history can resolve it.
