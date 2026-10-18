# Archive recovery and observation coverage

## Why

The CLI cannot fill an interior archive gap once newer observations exist.
MCP backfill returns an exact cursor but accepts only calendar dates, so a
continuation across a daylight-saving transition can skip part of a day.
Observation series also report zero rain and lightning when those sensors
have no readings. Users need precise recovery and visible sensor coverage.

## What Changes

- Add bounded CLI collection with paired inclusive `--start` and `--end` dates.
- Accept the exact exclusive `before_epoch` cursor in `backfill_archive`.
- Add rain and lightning reading counts to observation-series buckets and use
  them to distinguish missing readings, measured zero, and partial coverage.
- Document replay, continuation, limits, and compatibility.

## Capabilities

### New Capabilities

- `command-interface`: explicit date-range archive repair through the CLI.

### Modified Capabilities

- `mcp-service`: exact backfill continuation and truthful series display.
- `archive-storage`: sensor coverage counts in downsampled observation series.

## Impact

CLI flags, MCP input and series output gain additive fields. Existing numeric
series totals keep their types. No storage migration or runtime dependency is
needed. See [ADR 0008](../../../docs/adr/0008-archive-recovery-and-coverage.md).
