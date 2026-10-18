## Context

The archive stores `report_interval_min`, `wind_interval`, `precip_type`,
`device_id`, and `source`. Existing observation reads expose only sensor values.
Solar reports use radiation multiplied by valid report duration. The app resets
all table controls in `receiveResult`, even after the same read-only request.

## Decisions

Use `export --metadata` to append `device_id`, `report_interval_min`,
`wind_interval_s`, `precip_type`, and `source`. Preserve stored zeroes and NULLs.
Do not convert metadata with `--units us`. Keep one observation per output row;
do not insert a metadata preamble. Retain the streaming read-only path and range
ordering. Add a distinct store row type rather than widening `model.Obs` and
changing unrelated tool JSON. Older schemas expose absent metadata as missing.
Only allowlisted column names may enter schema-aware SQL. Bound source text
before loading it and use JSON-compatible escaping.

Preserve table selection by dataset name and compatible ordered column identity.
Keep search, sort direction, and page, then clamp the page against refreshed
filtered rows. Preserve current controls at result arrival, including edits made
while refresh is pending. Restore focused table controls and search selection
when compatible. New tool input starts a new context. A missing dataset or
changed schema resets controls. Failed refreshes retain the previous result and
controls. Chart selection still resets with a new result.

Version 0.4.0 identifies this feature batch. Keep historical 0.3.0 records intact,
align active binary, plugin, embedded app, and user guidance, and preserve the
existing nonpublishing release process.

## Scope and Fallback

No export import, schema migration, derived energy field, archive-wide table
search, persistent browser storage, or cross-session state is included. If a
metadata field cannot be read truthfully, return missing or a classified error;
never invent a duration or source. A smaller fallback retains only reporting
interval metadata and search persistence, with the same compatibility rules.

## Verification

Run focused export/store and version tests, browser refresh regressions, strict
specification checks, app bundle checks and screenshots, and `make verify`.
Exercise missing and zero values, source escaping and bounds, old schemas,
cancellation, callback errors, filtering while refresh is pending, schema
changes, duplicate SQL columns, page clamping, failed calls, and stale replies.
Use synthetic data and independently review the integrated diff.
