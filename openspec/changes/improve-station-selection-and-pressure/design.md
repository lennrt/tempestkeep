## Selection and Scope

Prioritize the reproduced pressure defect, then multi-station MCP operation and
its device-discovery view. Include the small reproduced report-validation defect.
Defer rain-summary coverage and comfort aggregation until their output semantics
have a separate design. A generic recursive result viewer adds complexity without
improving the specific device-selection workflow.

## MCP Device Selection

An explicitly supplied positive `--device-id` overrides the environment, including
an invalid environment value. Otherwise use `TEMPEST_DEVICE_ID`; empty or zero
means automatic discovery, preserving the existing environment convention.
Reject malformed, negative, overflowing, or explicitly zero flag values before
archive/API work. Validate explicit flags before dotenv access. Internal options
use zero for discovery and reject negative values before opening an archive.

Resolve valid selected IDs lazily against the token's station list. Require the
exact device to have type ST. Cache only a successful station/device choice;
unknown or non-Tempest devices never fall back to another live device. Keep
startup tolerant of live outages. An empty archive file can still be initialized
at startup, but failed selection cannot bind a device or alter observations and
progress. Existing archive mismatch protection remains effective.

The selected device controls collection and the default station. Live observations
and forecasts remain station-level. Explicit tool `station_id` overrides affect
only that read and leave the configured default intact. `list_stations` still
lists accessible stations. Without a token, selection does not restrict archive
reads. Existing archive fallback in `current_conditions` remains clearly labeled;
failed live resolution leaves `station_info` without live identity fields.

## Device Inspection

For `list_stations`, derive a Devices table with station ID/name and device
ID/type/serial. Retain parent association, duplicate device entries, missing
fields, and original results. Use a fixed column schema and existing table
controls. Display and search identifier numbers without thousands separators.
Mark numeric IDs outside the browser's exact integer range instead of offering
rounded identifiers for copying. Text IDs and raw tool results remain unchanged.
Bound traversal and displayed device entries to 2,000, with a visible omission
notice. Do not infer missing devices or add tool calls, storage, or network access.

## Pressure Samples

Use indexed predecessor and successor probes around latest pressure epoch minus
the window. Compare only earlier samples separated from the latest by at least
half the requested window; use exact integer bounds with a positive span. Pick
the nearest eligible sample and prefer the older sample on ties. Keep default
and maximum windows, actual elapsed span, normalized rate, and freshness rules.
Do not introduce a full-history distance sort. Return no trend when no eligible
pair exists. Preserve cancellation and classified archive errors.

## Report Ranges and Verification

Stats and export share date parsing. After resolving inclusive local dates, reject
negative epochs, unsupported maximum epochs, reversed ranges, and spans above the
existing store limit before dotenv or archive access. Keep DST and valid future
ranges. Test usage status and no filesystem side effects.

Use synthetic multi-station requests, exact pressure fixtures, invalid-range
commands, and browser interactions. Run focused regressions against old behavior,
full `make verify`, strict specifications, app bundle/browser/screenshot checks,
and independent review. A smaller fallback delivers pressure correctness and
MCP selection without derived app tables. Publication remains in PR #9.
