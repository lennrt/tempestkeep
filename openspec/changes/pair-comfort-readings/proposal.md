# Pair comfort readings

## Why

Independent quarter-hour means can invent heat index, wind chill, and dew point
from observations that never carried the required sensor pair.

## What Changes

Use means over complete temperature/humidity or temperature/wind observations
for each derived metric. Preserve 15-minute buckets, local days, measured zero,
and the mild-temperature fallback. Record the decision in ADR 0013.

## Capabilities

### Modified Capabilities

- `archive-storage`: require paired sensor inputs for derived comfort readings.

## Impact

Historical derived values can change for partial sensor coverage. Public
signatures, JSON fields, storage, dependencies, and read-only access stay stable.
