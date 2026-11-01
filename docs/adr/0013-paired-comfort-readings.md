# ADR 0013: Paired comfort readings

Status: Proposed

## Context

Independent sensor means can combine a temperature-only row with a
humidity-only row and report a heat index or dew point never supported by a
complete observation. The same problem affects cold temperature and wind.

## Decision

Within each existing 15-minute epoch bucket, calculate temperature and humidity
means from exactly the rows carrying both values. Use those paired means for
dew point and, when their temperature is at least 80°F, heat index. Calculate
temperature and wind means from exactly the rows carrying both values; when
their temperature is at most 50°F, use them for wind chill. Keep the existing
mild-air fallback from all available temperature samples when their mean lies
strictly between 50°F and 80°F. Each available apparent-temperature candidate
contributes to the daily minimum and maximum. Pairing does not cross buckets.
Measured zero remains a value. Missing required pairs produce no corresponding
derived reading. A bucket with temperature still counts as an observed day.

## Consequences

No schema, dependency, public signature, or JSON field changes. Historical
comfort values can change when sensors have different sample coverage.
Quarter-hour aggregation remains an estimate; pairing does not turn it into
instantaneous extremes. Reads remain on the read-only Store connection.
