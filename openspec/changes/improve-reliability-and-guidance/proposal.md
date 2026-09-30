# Improve reliability and guidance

## Why

Several boundary cases disagree with documented behavior. A busy SQLite reader
can leave a checkpoint incomplete without reporting failure. Bounded archive
calls can exceed their requested work or lose progress toward history exhaustion.
Some tool results and instructions describe the wrong station, range, or capability.

## What Changes

- Require a complete checkpoint before the CLI creates a backup.
- Correct API response validation, cache behavior, retry arithmetic, and series limits.
- Preserve bounded archive progress across calls and restarts.
- Make station selection, date ranges, CLI arguments, and capability guidance consistent.
- Correct sparse solar estimates, fractional wind sectors, and missing-sensor analysis.
- Preserve local calendar dates and independent sensor readings in terminal displays.
- Validate complete setup output before replacing an existing configuration file.
- Add concrete MCP, SQL, and CLI examples with explicit units and boundary rules.
- Add OpenSpec requirements, a repeatable validation command, and a dedicated CI job.
- Repair documentation checks for fenced examples and encoded link paths.

## Capabilities

### New Capabilities

- `command-interface`: specify existing command, display, and setup boundaries.

### Modified Capabilities

- `archive-storage`: checkpoints, bounded queries, sensor analytics, and resumable progress.
- `weather-api`: malformed response rejection, cache acceptance, and retry arithmetic.
- `mcp-service`: truthful station labels, bounded archive calls, and actionable guidance.
- `contributor-workflow`: OpenSpec validation and reliable documentation examples.

## Impact

The change affects archive collection, API reads, MCP tools, CLI validation,
documentation, and development checks. Existing Go package signatures, JSON
shapes, and observation columns remain stable. Private archive metadata can grow
to preserve collection progress. Node.js is required only for specification work.

[ADR 0005](../../../docs/adr/0005-reliability-and-specifications.md) records
the compatibility decisions. The pull request remains a draft for review.
