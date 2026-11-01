# Improve result exploration

## Why

Dataset changes remove the focused keyboard control. Long SQL strings are
truncated before both display and search, hiding received values. Daily forecast
rows have no summary cards although their temperatures and rain chances are available.

## What Changes

- Restore focus to the selected dataset control after local rendering.
- Search longer SQL strings and expose a keyboard accessible cell inspector,
  bounded to 65,536 characters per cell with visible limit notices.
- Summarize daily forecast highs, lows, rain chance, and returned days.

## Capabilities

### Modified Capabilities

- `mcp-service`: bounded accessible result exploration.

## Impact

Changes affect only app display and local interactions. Tool fields, schemas,
read-only boundaries, units, and raw results remain unchanged. No new dependency
or tool call is added. See [ADR 0015](../../../docs/adr/0015-bounded-result-exploration.md).
