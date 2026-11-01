## Why

CLI archive repair needs gap boundaries. Existing stats does not expose gaps or
saved backfill completion, and MCP status requires an MCP client.

## What Changes

- Add read-only status in text and JSON through existing archive readers.
- Report coverage, freshness, saved completion, and ten largest internal gaps.
- Explain empty archives and the limits of stored observation coverage.
- Bound metadata projections across UTF-8 and UTF-16 databases, then enforce
  the writer's existing 4,096-byte UTF-8 limit.
- Evaluate one matching metadata value once before guarding its projection,
  including when a legacy schema exposes metadata through a view.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `command-interface`: offline status for terminal users and scripts.

## Impact

Add one command and JSON output shape. No public Go API, schema, dependency,
network operation, or writer is added. See ADR 0016.
