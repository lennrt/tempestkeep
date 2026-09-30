# ADR 0005: Enforce archive boundaries and track behavior with OpenSpec

## Status

Proposed in the reliability and documentation draft pull request.

## Context

The audit reproduced incomplete checkpoint success, invalid API cache entries,
unaligned query limits, and metadata that did not always survive interruption.
MCP work budgets, calendar bounds, station labels, and some instructions also
disagreed with their stated behavior. These defects cross storage and protocol
boundaries even though their fixes retain existing public field shapes.

The project needs persistent requirements that connect each expected behavior
to a reproducible test. OpenSpec provides the document format and validation.
SimpleEnglish provides guidance for clearer instructions.

## Decision

Require SQLite to report a complete checkpoint before a file backup proceeds.
Read the PRAGMA result row and report ErrArchiveIO for busy or incomplete work.
Preserve the existing instruction to stop other writers before copying a database.
A completed checkpoint does not make a later file copy safe against another writer.

Reject malformed JSON, non-success API status envelopes, unsupported observation
types, mismatched device identities, and observations outside the requested range.
Check optional identity fields when present. Keep absent fields compatible with
existing fixtures and clients. Do not cache responses that fail these checks.
Retain sanitized diagnostics and existing error classifications.
Require the endpoint's collection field before accepting an empty response.
For compatibility, an explicit null collection remains an empty collection.
Bound array lengths, including nested arrays, before allocating typed values.
Check every occurrence of a collection key so duplicate keys cannot hide an
oversized earlier collection from the allocation limit.

Bound Retry-After arithmetic before converting seconds to a duration. Count
series limits by the epoch-aligned buckets that the requested range intersects.
For arbitrary read-only SQL, apply SQLite limits on the dedicated query
connection before result expressions allocate large values. Restore the previous
limits before returning the connection to the pool. Preserve bounded output and
supported error classifications.
Reject non-finite SQL numbers before they reach JSON output.

Keep fractional wind directions when assigning compass sectors. Compute the
calm fraction from all observations with a valid wind speed, including rows
without a direction. Directional sector percentages use non-calm rows with a
known direction, so unknown headings do not create a false north sector.

Estimate solar energy from each irradiance reading and its positive report
interval. Missing or zero intervals do not contribute energy, but their sensor
peaks remain available. Include and group each interval by its sample timestamp,
as rain reports already do. Do not extrapolate sparse samples across a complete
15-minute bucket. This is an interval-based estimate, not a direct energy
measurement. It does not clip intervals at query boundaries or remove overlap.

Break dry and storm-free spells when a day has no relevant sensor readings.
A recorded zero remains a valid reading. Calculate apparent temperatures only
when the sensors required for the temperature band are present. Retain the
existing day counts and public fields; do not invent a daily completeness threshold.

Normalize API labels before plain terminal output. Quote generated Windows
commands for literal PowerShell paths. Label a forecast as Today only when its
date is today. Plot the requested local day even when its first UTC-aligned
bucket starts on the previous local date.
Count calendar days with calendar arithmetic across daylight-saving transitions.
Show rain and gust readings even when a day's temperature is missing. Distinguish
an empty selected range from measured calm or no detected weather events.
Validate the complete generated dotenv content before replacing an existing file,
so setup cannot write a file that the configuration parser will reject.

Save seed intent before the first backfill fetch. A stored cursor at epoch zero
means that backfill reached its terminal boundary. Resume that cursor without
requiring an observation watermark. Persist progress after successful chunks so
callback failures and interruption cannot make the next run reject its own state.

Make MCP max_days a hard upper bound on requested history. Preserve empty-history
progress between bounded calls with private metadata tied to the saved cursor.
An absent or stale record starts conservatively. Serialize MCP write workflows
within one server, and permit cancellation while a call waits for its turn.
This does not promise coordination between separate server processes.

Persist successful forward scan windows for bounded MCP sync calls. Anchor the
private cursor to the newest stored observation, and discard it when another
writer changes that watermark. This lets later calls cross empty history beyond
one work budget. A failed chunk or metadata save remains safe to replay.
After reaching the requested present time, reset the hint to the watermark so
later calls recheck the empty tail for observations that the API published late.

Resolve forecast station metadata from the requested accessible station.
Combine live station identity with archived readings only when the archive's
device belongs to that station. Missing identity leaves the archive unlabeled.
Add an archive pressure trend to live conditions only when the devices match.
For either conditions source, require its latest pressure reading to be no later
than the displayed observation and less than one hour older. This reuses the
existing archive freshness interval. Normalize pressure_trend_3h_inhg to three
hours from the actual sample span. The dedicated pressure_trend tool keeps its
historical timestamps, span, actual change, and normalized rate.
With an explicit summary start, treat an omitted end as the present time.
Archive read end dates include the named day and exclude the next local midnight,
including daylight-saving transitions. Backfill end dates exclude the named
midnight. Validate date strings before live discovery or collection.

Reject unexpected CLI positional arguments with usage status 2 before command
I/O. Reject interval values that overflow time.Duration. Give stats and export
the existing SIGINT and SIGTERM cancellation behavior.

Add pinned OpenSpec tooling, baseline requirements, active change artifacts, and
strict CI validation. Keep Node.js out of the application build and runtime.
Run native pure-Go tests and documentation checks on macOS and Windows in CI.
Use SimpleEnglish Plain guidance for prose without claiming formal STE compliance.
Keep technical identifiers, commands, facts, and units intact.

Update the existing go-localereader dependency to its license-only upstream
commit. Its Go source is unchanged. The complete MIT license allows the existing
scanner to inspect the Windows dependency without a new exception.

## Compatibility

No public Go signature, observation-table schema, MCP tool name, or JSON field
shape changes. Private metadata gains mcp_backfill_empty_span and mcp_sync_cursor
records. Older binaries ignore them and retain their former collection behavior.

Previously ignored CLI arguments now fail. Unsupported or inconsistent API
responses now fail instead of appearing as empty history or valid observations.
Busy checkpoints now prevent backup creation. Small backfill calls stop at their
declared budget and remain resumable. Forecast labels and open-ended summary
ranges change to match the request.
Solar energy estimates and wind percentages can change for sparse or incomplete
sensor data. These corrected values retain their existing units and field names.
Spells no longer bridge days without relevant readings. Apparent-temperature
extremes omit samples with missing required sensors.

## Validation

Use synthetic regression tests for checkpoint contention, replay, interrupted
seed collection, terminal cursors, query limits, retry overflow, and malformed
API envelopes. Test both direct API use and collection through a custom fetcher.

Test MCP calls with one-day and non-multiple budgets, process restarts, callback
failures, explicit floors, and cancellation while waiting for the writer.
Test station selection and calendar boundaries through real MCP transports.
Run documented examples against the synthetic API and an offline archive.

Run the repository checks and strict OpenSpec validation. Record unavailable
checks separately from successful checks in the pull request evidence.
