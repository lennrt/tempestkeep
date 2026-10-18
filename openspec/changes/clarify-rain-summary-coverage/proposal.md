## Why

Rain summaries collapse missing sensor readings into a numeric zero. All-missing
rain and measured zero can produce the same total, while partial readings look
complete. The user requests clear rain coverage and a feature-focused docs guide.

## What Changes

- Return a nullable rain summary total and explicit observation/readings counts.
- Distinguish empty, missing, partial, and fully reported sample sets in CLI,
  JSON, MCP, and the scalar app view.
- Highlight multi-station selection, interactive exploration, archive recovery,
  exports, and offline workflows in the documentation overview.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `archive-storage`: rain summary totals preserve missingness and sample coverage.
- `command-interface`: text and JSON reports distinguish unknown rain from zero.
- `mcp-service`: rain statistics expose coverage and a stable rainfall headline.

## Impact

`RainStats.TotalIn` changes from `float64` to `*float64`. The `total_in` JSON value
becomes number-or-null and remains present. Two count fields are added. Go callers
must check for nil; strict JSON consumers must accept null and the new fields.
Other rain aggregates, archive schemas, dependencies, and thresholds stay unchanged.
Version 0.4.0 remains the development target. Delivery updates PR #9 without merging.
