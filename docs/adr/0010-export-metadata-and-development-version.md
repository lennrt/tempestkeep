# ADR 0010: Export metadata and development version

Status: proposed

## Context

The archive stores observation intervals and provenance that existing exports
omit. In particular, solar-energy estimates cannot be reproduced without the
report interval. Existing scripts may depend on the current export columns.
The improvement batch also adds user-facing features beyond the proposed 0.3.0
MCP Apps baseline.

## Decision

Add `export --metadata`. Append device ID, report interval in minutes, wind
interval in seconds, precipitation type, and stored source. Preserve defaults,
missing values, and numeric zeroes. Metadata retains its units in US exports.
Use a separate additive store row type and streaming iterator, without changing
`model.Obs` or unrelated MCP JSON. Older missing columns yield missing values.
Bound source text before allocation and use format-correct escaping. No archive
migration is needed.

Version this feature batch as 0.4.0 in active development and plugin metadata.
Preserve build overrides and historical version records. Keep release and
plugin publication as separate maintainer actions.

## Consequences

Default exports stay compatible. Opt-in CSV consumers must accept five added
columns, and JSON consumers must accept optional fields. Exports remain analysis
files rather than restorable archive backups. Energy calculations must exclude
missing or invalid intervals; they must not assume continuous sensor coverage.
The API snapshot and regression tests record the additive store contract.
The version bump does not publish or merge the PR.
