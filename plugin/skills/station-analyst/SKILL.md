---
name: station-analyst
description: >-
  Analyze a local WeatherFlow Tempest archive through TempestKeep MCP tools.
  Use for station records, extremes, comparisons, weather patterns, and questions
  about gardening, sunlight, or outdoor activities that depend on station data.
---

Use data from the user's station. The archive stores one device's observations.
Choose the smallest query that answers the question. Discover available tools
before using a workflow that needs live access or archive writes.

Before comparing live weather with archive history, call `station_info`.
If it omits live station identity, describe the archive separately. Do not
assume that a configured archive belongs to the selected live station.

## Select a tool

| Question | Tool |
|---|---|
| Coverage, freshness, and large gaps | `archive_status` |
| Extremes across the archive | `records` |
| This calendar date across years | `this_day_in_history` |
| Monthly or yearly comparisons | `period_summary` |
| Daily values within a range | `daily_summary` |
| Wind direction and speed distribution | `wind_rose` |
| Typical temperature or wind by local hour | `climatology` |
| Heating, cooling, or growing degree-days | `degree_days` |
| Frost, hot-day, and tropical-night counts | `climate_indices` |
| Observed pressure change | `pressure_trend` |
| Rain totals and wet or dry spells | `rain_stats` |
| Monthly baseline across available years | `climate_normals` |
| Solar radiation and UV | `solar_stats` |
| Missing sensor readings | `sensor_health` |
| A time series for a chart | `get_observations` |
| A query that no dedicated tool supports | `query_sql` |

Before the first `query_sql` call, read `tempest://archive/schema` and
`tempest://archive/data-dictionary`. Use the resource's exact column names.
If a result reports truncation, narrow or aggregate the query.

## Keep units and dates explicit

Name the units in every answer. Raw SQL returns SI values: °C, m/s, mm, and
millibars. Most history tools convert those measurements to °F, mph, inches,
and inHg. Solar radiation remains W/m².

Treat `epoch` as UTC seconds. Use dedicated calendar tools for local dates.
Calendar tools use the server's process timezone, which need not match the
station. Do not substitute a fixed UTC offset across daylight-saving changes.

For history queries, `end` includes the full local end day. For
`backfill_archive`, `end` is exclusive local midnight. Use the returned range
when describing results.

Rain and lightning values are increments per report interval. Sum `rain_mm`
and `strike_count` for totals. Do not average them or multiply them by interval
length. Use `MAX(wind_gust)` for peak gusts and `AVG(wind_avg)` for mean wind.

For `wind_rose`, sector percentages use non-calm readings with a known
direction. The calm percentage uses every reading with a wind speed.
Missing directions therefore reduce the evidence for directional comparisons.

Treat solar insolation as an estimate from irradiance and reported duration.
Missing or zero durations contribute no energy, but sensor peaks remain
available. Each whole interval belongs to its observation timestamp's local
day. The estimate does not clip boundaries or subtract overlaps.

For `get_observations`, gust and UV values are bucket maxima. Wind and pressure
are bucket means. Rain and lightning are bucket totals. Read the applied bucket
width before describing a chart's resolution.

## Account for missing observations

Call `archive_status` before historical comparisons. Its gap list reports at
most ten gaps longer than an hour. A lack of listed gaps does not prove that
all minutes or sensors are present.

Treat a missing field or SQL `NULL` as unavailable. Use `sensor_health` for
continuous sensors. Count non-null `rain_mm` or `strike_count` values in SQL
when an event total or spell depends on coverage.
Do not replace missing observations with zero. Some summary totals use zero
when sensor readings are absent, so compare them with coverage evidence.

A day without rain or strike readings breaks the corresponding spell.
One valid reading lets a day qualify, so spells do not prove complete coverage.
`comfort_stats` omits hot apparent temperatures without humidity and cold
apparent temperatures without wind. Its extremes use 15-minute means.

Include observation counts when ranking periods. Compare the same elapsed
dates for a partial current month and earlier years. Describe `records` as
archive records, because missing history can hide a larger event.

Describe `climate_normals` as the baseline of the available archive. Do not
call it a standard long-term climate normal. Explain how partial years,
gaps, or station changes limit an inferred temperature trend.

Use `forecast` for expected weather when live access exists. A pressure trend
alone does not establish that a storm will occur. Archived pressure is station
pressure, while live conditions can use sea-level pressure.
Live conditions include an archive trend only when station identity matches.
For either source, the latest pressure reading must be no later than the
displayed observation and less than one hour older. An absent trend does not
mean steady pressure. `pressure_trend_3h_inhg` is a normalized three-hour rate;
the separate `pressure_trend` tool exposes the actual historical sample span.

## Worked examples

For "Which day had the strongest gust in 2025?", call `daily_summary`:

```json
{"start": "2025-01-01", "end": "2025-12-31"}
```

Rank days by `peak_gust_mph` and include `obs`. For the selected day, call
`get_observations` with matching `start` and `end` and `bucket_minutes: 60`.
Report peak gust separately from average wind. If the archive lacks part of
2025, state the covered dates.

For "Was June wetter this year?", call `period_summary` for June in each year.
Use explicit first and last dates and `period: "month"`. Compare `rain_in`,
`rainy_days`, `days_observed`, and `obs`. If either month is incomplete, state
that the totals cover different amounts of data.

For "When does my yard get the most sun?", use `solar_stats` for the range.
Use `get_observations` or bounded SQL when hourly detail is needed. Describe
the station's measured sunlight. Do not claim that it measures shade across
the whole yard or electrical output from solar panels.

For "What does the station show about spring frost?", query daily lows for
spring dates across available years. Keep the frost threshold explicit.
Compare recent lows with the forecast when live access exists. State the
archive's coverage instead of promising a frost-free planting date.

## Write the answer

Give the answer first, followed by the supporting numbers, dates, and units.
Use small tables for rankings. Distinguish observations, calculated values,
and forecasts. For current conditions, include the source and timestamp.

If the archive cannot support the answer, state the missing range or sensor
readings. Offer `/tempestkeep:build-archive` only when historical collection
can address the gap. Do not imply that backfill can recover data absent from
the upstream API.
