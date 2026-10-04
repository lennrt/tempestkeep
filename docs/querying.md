# Query the archive

An archive contains observations from one Tempest device. It records what the
station reported and what collection retrieved. Missing rows and missing sensor
readings limit every comparison.

Use the MCP history tools for common questions. Use `query_sql` when a tool
cannot express the query. The [MCP guide](mcp.md) explains connection,
capabilities, and tool argument limits.

## Choose a query

| Question | Tool and scope |
|---|---|
| What data is available? | `archive_status` for coverage, freshness, and the ten largest gaps |
| Which day had the strongest gust? | `daily_summary` for the range, ranked by `peak_gust_mph` |
| What happened within that day? | `get_observations` for a series of time buckets |
| How did two months compare? | `period_summary` with `period: "month"` and explicit dates |
| What are the recorded extremes? | `records` for the whole archive |
| What happened on this date in other years? | `this_day_in_history` with `date: "06-03"` |
| What is the usual wind direction? | `wind_rose` for 16 compass sectors |
| What is the usual temperature by hour? | `climatology` for local hours of the day |
| Are continuous sensor readings missing? | `sensor_health` for the requested range |
| What changed in station pressure? | `pressure_trend` for a trailing window ending at the latest archived pressure reading |

A tool's range is part of the answer. For example, a record is the largest
stored value, not a guarantee about the station's complete lifetime. A pressure
trend describes observations. It does not replace a forecast.

## Dates and timezones

Stored `epoch` values are Unix seconds in UTC. Calendar tools group rows in the
process timezone, which defaults to the host timezone. They do not infer the
archive's timezone from its device.

On Unix-like systems, set `TZ` before starting the process:

```sh
TZ=America/Los_Angeles bin/tempestkeep stats --db /absolute/path/to/tempest.sqlite
```

Native Windows uses the host timezone for Go's local time. Setting `TZ` alone
does not select the calendar timezone there. Use a host timezone that matches
the station or run calendar reports in a Unix-like environment with `TZ` set.

For history reads, `start` includes local midnight and `end` includes the full
local end day. For `backfill_archive`, `end` instead excludes that local
midnight. See the [backfill example](mcp.md#build-an-archive).

A daylight-saving transition can make a local day 23 or 25 hours long. Do not
assume that every local day contains 1440 one-minute observations. Prefer the
calendar tools to SQL that applies a fixed UTC offset across changing dates.

`get_observations` uses fixed-duration buckets aligned to the Unix epoch.
Its timestamps identify bucket starts. A large bucket does not necessarily
start at local midnight, and the first or last bucket can contain only part
of its usual range. Use `daily_summary` for local calendar days.

## Units and aggregation

Read `tempest://archive/schema` and `tempest://archive/data-dictionary` before
the first SQL query. Raw SQL returns stored units. Most history tools convert
temperature, wind, pressure, rain, and lightning distance to US display units.
Solar radiation, UV, humidity, and other fields keep their documented units.

| Measurement | Stored column and unit | Common tool unit | Correct operation |
|---|---|---|---|
| Temperature | `air_temp_c`, °C | °F | Mean, minimum, or maximum for the stated question |
| Average wind | `wind_avg`, m/s | mph | Mean of observed values |
| Peak gust | `wind_gust`, m/s | mph | Maximum for a period's strongest gust |
| Station pressure | `pressure_mb`, millibars/hPa | inHg | Mean, minimum, maximum, or change |
| Rain | `rain_mm`, mm per report interval | Inches | Sum of reported intervals |
| Lightning | `strike_count`, count per report interval | Count | Sum of reported intervals |
| Solar radiation | `solar_wm2`, W/m² | W/m² | Mean or maximum, as labeled |
| Wind direction | `wind_dir`, degrees from north | Compass sector | Circular analysis, such as `wind_rose` |

Do not average wind direction as ordinary numbers. For example, northward
directions of 350° and 10° do not average to south. `wind_rose` reports where
wind comes from and excludes calm samples from directional percentages.
Its sector percentages use non-calm observations with a known direction.
Its calm percentage uses all observations with a wind speed, including those
without a direction. Calm means a speed below 0.5 m/s, about 1.1 mph. Fractional
directions retain their precision: 11.25° belongs to the NNE sector.

Rain and lightning increments cover each row's report interval, which can
differ from one minute. Sum the increments without multiplying by interval
length.

`solar_stats` estimates solar energy by summing each `solar_wm2` reading times
its positive `report_interval_min`, times 60, divided by 1,000,000. The result
is MJ/m². For example, 100 W/m² over one reported minute contributes 0.006 MJ/m².
Intervals are limited to 1440 minutes. Missing or zero intervals contribute
no energy, but their sensor readings still contribute to peaks.

The observation timestamp selects the query range and local day for each
whole interval. The estimate does not clip intervals at those boundaries or
subtract overlaps between intervals. Gaps receive no estimated energy.
`avg_daily_insolation_mj` averages only days with a solar reading and a usable
interval. `days_observed` counts all days with stored observations, including
days without solar readings.

Treat insolation as an estimate from reported intervals. A station's sunlight
measurements do not measure a solar panel's electrical output.

Live conditions prefer sea-level pressure when available. Archived pressure
is station pressure. Do not interpret a switch between these sources as a
weather change. For a consistent pressure comparison, use archive tools.

Live `current_conditions` includes an archive pressure trend only when the
archive's device belongs to the live station. For either live or archive-sourced
conditions, the latest pressure reading must be no later than the displayed
observation and less than one hour older. A recent row without pressure does
not make an older pressure trend current. An absent trend does not mean steady
pressure.

`pressure_trend_3h_inhg` is a rate normalized to three hours from the actual
sample span. It is not necessarily a measured change over exactly three hours.
The separate `pressure_trend` tool retains the historical samples' time, span,
and actual change, along with the normalized rate.

`get_observations` preserves temperature means, minima, and maxima. It uses
mean wind speed, maximum gust and UV, and total rain and lightning per bucket.
An hourly `gust_mph` is the largest stored gust in that bucket.

## Coverage and missing readings

Start an analysis with `archive_status`, then inspect the requested range.
`observations` counts stored rows. It does not count valid readings for every
sensor. A row can contain a temperature and no rain reading.

In SQL, `NULL` means that the sensor value is missing. `COUNT(*)` counts rows,
while `COUNT(air_temp_c)` counts rows with a temperature reading. `AVG`, `MIN`,
`MAX`, and `SUM` skip missing values. `SUM` returns `NULL` when there are no
non-null input values.

Some history totals use zero when no valid increments exist. Compare those
totals with SQL reading counts for `rain_mm` or `strike_count`. `sensor_health`
covers continuous sensors, not these interval counts. A returned zero alone
does not prove that a working sensor recorded no rain or lightning.

For rain and lightning spells, an absent day or a day without the relevant
sensor reading breaks the run. One valid reading lets a day qualify, so a
reported spell does not imply complete sensor coverage throughout every day.

`comfort_stats` calculates apparent temperature from 15-minute sensor means.
At or above 80°F, it needs humidity. At or below 50°F, it needs wind speed.
Between those temperatures, it uses air temperature alone. Missing required
readings leave the apparent value unavailable. These calculated extremes
differ from instantaneous sensor extremes. Its `days_observed` counts days
with temperatures, even when other required readings are absent.

Empty calendar periods and empty series buckets are absent. Do not fill them
with zero and call the result observed weather. A day with a few observations
can still contribute to a daily summary. Compare sample counts and covered
dates before ranking days, months, or years.

`climate_normals` describes the available archive. It does not establish a
standard long-term climate baseline. `temperature_trend` fits the recorded
series. Gaps, partial years, and changes to station placement can affect that
fit. Report the available span and evidence before interpreting a trend.

## Worked tool calls

These examples use tool names followed by argument objects. Replace the dates
with a period covered by your archive. The client provides the JSON-RPC wrapper.

For the strongest recorded daily gust in calendar year 2025, call
`daily_summary`:

```json
{"start": "2025-01-01", "end": "2025-12-31"}
```

Rank returned days by `peak_gust_mph`, excluding absent gust fields. Keep each
day's `obs` count with the result. Use `get_observations` for hourly detail on
the selected day:

```json
{"start": "2025-06-03", "end": "2025-06-03", "bucket_minutes": 60}
```

The dates in the second call are illustrative. Use the date from the first
result. Distinguish the peak gust from the average wind speed in your answer.

For June 2024 and June 2025, call `period_summary` once for each month:

```json
{"period": "month", "start": "2024-06-01", "end": "2024-06-30"}
```

```json
{"period": "month", "start": "2025-06-01", "end": "2025-06-30"}
```

Compare rain totals, temperature values, observed days, and row counts.
If either month is incomplete, state that limitation. For a current partial
month, compare the same elapsed dates in each year.

## Worked SQL queries

Pass SQL through the `sql` field of `query_sql`. For example, this call
retrieves ten recent rows with temperatures:

```json
{
  "sql": "SELECT epoch, air_temp_c FROM obs_st WHERE air_temp_c IS NOT NULL ORDER BY epoch DESC LIMIT 10",
  "max_rows": 10
}
```

The temperature stays in °C. The following SQL examples can also be passed in
that field. They use explicit UTC boundaries so their date meaning does not
depend on SQLite's local-time behavior.

To measure rain coverage and totals for June 2025 in UTC:

```sql
SELECT COUNT(*) AS stored_rows,
       COUNT(rain_mm) AS rain_readings,
       SUM(rain_mm) AS rain_mm,
       ROUND(SUM(rain_mm) / 25.4, 3) AS rain_in
FROM obs_st
WHERE epoch >= CAST(strftime('%s', '2025-06-01T00:00:00Z') AS INTEGER)
  AND epoch < CAST(strftime('%s', '2025-07-01T00:00:00Z') AS INTEGER);
```

The end boundary is exclusive. If `rain_readings` is zero, report that rain
data is unavailable. For a station-local calendar month, use `period_summary`
instead of reusing these UTC boundaries.

To rank five observed UTC days by peak gust, with average wind for context:

```sql
SELECT date(epoch, 'unixepoch') AS utc_day,
       ROUND(MAX(wind_gust) * 2.2369362921, 1) AS peak_gust_mph,
       ROUND(AVG(wind_avg) * 2.2369362921, 1) AS mean_wind_mph,
       COUNT(wind_gust) AS gust_readings,
       COUNT(wind_avg) AS wind_readings
FROM obs_st
WHERE epoch >= CAST(strftime('%s', '2025-01-01T00:00:00Z') AS INTEGER)
  AND epoch < CAST(strftime('%s', '2026-01-01T00:00:00Z') AS INTEGER)
GROUP BY utc_day
HAVING COUNT(wind_gust) > 0
ORDER BY peak_gust_mph DESC, utc_day
LIMIT 5;
```

This query ranks UTC days, not station-local days. Use `daily_summary` for
the local-day version. Means weight each available row equally, so uneven
report intervals or gaps affect the result.

SQL results contain column names and row arrays. If `truncated` is true,
the result omits further rows. Use explicit ordering and a smaller range or
aggregate instead of treating a truncated result as complete.

SQL results must use finite numeric values that JSON can represent. SQLite
also limits individual values, encoded rows, and column counts.
See the [MCP limits](mcp.md#limits-and-failure-behavior) before requesting large
expressions or result sets.
