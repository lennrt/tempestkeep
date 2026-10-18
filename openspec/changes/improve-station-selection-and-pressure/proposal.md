## Why

MCP always selects the first accessible Tempest device, even when CLI collection
uses a different device. Its station list already contains device identities,
but the app hides them behind an item count. Sparse pressure history also uses
only a reading before the requested window, despite promising the nearest
reading. This can reverse the reported tendency. Invalid report dates currently
reach archive access before the command reports their unsupported range.

## What Changes

- Add explicit MCP device selection through a flag and existing environment key.
- Derive a bounded, searchable devices table with parent station identity.
- Compare the nearest eligible pressure readings on either side of the target.
- Reject unsupported stats/export epochs and spans before configuration or I/O.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `mcp-service`: selected default station/device and station device inspection.
- `archive-storage`: deterministic nearest pressure comparison with valid spans.
- `command-interface`: selection precedence and early report range validation.

## Impact

MCP configuration now honors `TEMPEST_DEVICE_ID`. Tool schemas, archive formats,
and dependencies remain unchanged. Pressure results can change where the old
sample choice was wrong or overlooked sufficient history. App device tables are
derived from received results without new calls. Version 0.4.0 remains the
unreleased target. Changes join the existing PR #9 without merging it.
