# ADR 0012: Missing rain summaries and sample coverage

Status: proposed

## Context

RainStats converts missing rain to zero through daily sum aggregation. Missing,
measured-zero, and partial samples can therefore produce identical summaries.
The user requests that missing readings stop appearing as zero.

## Decision

Use one canonical nullable total. RainStats.TotalIn becomes *float64, encoded as
number-or-null under the existing total_in key without omitempty. A total is nil
exactly when no rain readings exist. Add observations and rain_readings counts
from the existing daily aggregate pass. Positive counts preserve measured zero;
partial totals sum reported values without extrapolation.

Keep day counts, thresholds, wettest-day selection, and spell semantics unchanged.
Distinguish empty, missing, and partial results in CLI/MCP and the scalar app.
Keep the app's rainfall headline stable when missing and preserve original data.

## Consequences

Go callers must handle a pointer and check nil. JSON/MCP consumers must accept
null and two new count fields. This intentional contract change supersedes the
previous RainStats shape-stability decision; other rain result types do not
change. Count-only output was rejected because canonical zero remains misleading.
A second total field would duplicate meaning and burden consumers.

No schema migration, additional query pass, or dependency is needed. Coverage
counts describe stored samples, never continuous observation time. Daily, period,
and normal rainfall aggregates need their own coverage design. Version 0.4.0
remains the unreleased development target, and PR #9 remains open for review.
