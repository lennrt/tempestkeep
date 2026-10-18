# ADR 0008: Explicit archive recovery and event sensor coverage

- Status: proposed
- Date: 2026-10-18

## Context

CLI collection follows the newest observation and saved seed progress. It
cannot revisit a selected old gap. MCP explicit backfill has calendar-date
inputs but returns an epoch cursor, which can fall inside a daylight-saving
day. Observation series also coalesce missing event readings into zero totals.

## Decision

Add paired inclusive CLI range flags and an exclusive MCP `before_epoch` input.
Reuse existing collection and replay behavior. Explicit requests leave shared
seed and sync metadata alone. Bound CLI requests to 366 local calendar days,
and retain existing MCP work budgets. Validate bounds before external work.

Add exported `SeriesPoint.RainReadings` and `SeriesPoint.LightningReadings`
integer fields with JSON keys `rain_readings` and `lightning_readings`.
Keep existing numeric totals and field types. Counts distinguish unknown,
measured zero, and partial coverage. The MCP description and app explain this
contract; the app displays unknown event totals as missing values.

## Compatibility and consequences

No observation schema or configuration migration is required. Existing calls
retain their behavior, and existing keyed Go struct literals remain compatible.
Unkeyed external struct literals must include the added fields. Strict JSON
consumers must accept the added count fields. Numeric totals alone still do
not establish sensor coverage; consumers should use the counts.

An explicit CLI range ignores an environment seed floor and rejects an explicit
seed-floor flag. After failure, repeat the same range; already committed rows
are replay-safe. MCP callers repeat start and max_days and pass next_before as
before_epoch. The shared no-argument cursor remains separate.

Recovery adds no automatic scheduler, external write, or archive replacement.
Missing upstream data stays missing. Counts measure available samples, not
continuous sensor uptime. The existing privacy and backup constraints remain.
