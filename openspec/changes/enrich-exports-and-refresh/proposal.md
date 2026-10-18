## Why

Exports omit stored reporting intervals and provenance. A user cannot reproduce
the archive's solar-energy estimate from an ordinary export. Successful app
refreshes also clear table searches, sorting, and pagination, interrupting
repeated analysis of the same result.

## What Changes

- Add opt-in export metadata with device, reporting interval, wind interval,
  precipitation type, and source fields. Keep default exports unchanged.
- Add a read-only streaming archive iterator for these stored fields.
- Retain compatible table controls when a result refreshes. Reset them when
  the table structure or request context changes.
- Bump development and plugin version metadata from 0.3.0 to 0.4.0.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `command-interface`: optional export metadata with unchanged default output.
- `archive-storage`: streaming access to observation provenance and intervals.
- `mcp-service`: table exploration state survives compatible refreshes.
- `contributor-workflow`: consistent development version without publication.

## Impact

The Go store API gains an additive iterator and row type. The export flag adds
CSV columns or JSON fields only when requested. No archive migration, dependency,
network origin, or new tool call is needed. The version bump does not publish a
tag, release, or plugin. Changes join PR #9, which remains open.
