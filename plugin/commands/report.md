---
description: "Weather report from available live conditions, forecasts, and station history"
argument-hint: ""
---

Produce a weather report from the user's station with the `tempestkeep` MCP
tools. Discover the available tools first. Gather the data before writing,
and do not stream raw tool results.

1. Call `current_conditions`. Read `source`, `time`, and `age_seconds` before
   describing the reading as current.
2. If live tools are available, call `forecast` for the next three days.
3. If archive tools are available, call `station_info`, `archive_status`, and
   `this_day_in_history` for historical context.
4. If archive tools are available, call `records` to compare recorded extremes.

Write a short report with these parts when data is available:

- Latest conditions: temperature, apparent temperature, wind, and humidity.
- Forecast: each day's high, low, conditions, and rain chance.
- History: this calendar date across available years, with covered years and
  sample counts where useful.
- Observed extremes: a recent value near a recorded extreme, only when the
  measurements and units are comparable.

Use the units returned by each tool and round temperatures to whole degrees.
Include the observation's time and source. If the reading comes from an old
archive row, label it as archived conditions rather than current weather.

If `station_info` omits live station identity, describe archive history
separately from live conditions and forecasts. Do not claim that they describe
the same station. A missing pressure trend does not mean steady pressure.

If live tools are absent, omit the forecast and state that live access is not
configured. If archive tools are absent, omit historical comparisons and state
that history needs an archive. Refer to `/tempestkeep:setup` when configuration
can supply the missing capability.

Do not treat missing observations as evidence that an event did not occur.
Do not infer a dry spell from `records` alone. Use `rain_stats` and coverage
before reporting a count of rain-free days. Distinguish forecasts from stored
observations throughout the report.
