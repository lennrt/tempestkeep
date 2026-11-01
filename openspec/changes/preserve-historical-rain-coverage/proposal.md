## Why
Historical summaries turn missing rain into measured zero and dilute rain normals with months
that have no rain samples.

## What Changes
Preserve nullable rain totals and sample counts in daily, period and this-day history. Compute
rain normals only from rain-contributing months and expose their year count. Adapt terminal and
MCP consumers.

## Capabilities
### Modified Capabilities
- `archive-storage`: Preserve missing rain in historical analytical results.

## Impact
Rain totals and normal averages become nullable public Go pointers and JSON numbers or null.
Counts are additive. Archives do not change.
