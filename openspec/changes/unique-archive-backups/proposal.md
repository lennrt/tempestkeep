# Unique archive backups

## Why

Two successful collection runs in one second choose the same backup name. The
second run fails even though collection succeeded. Repeated backups need unique
names without replacing an existing snapshot.

## What Changes

- Include fixed nanosecond precision in new UTC snapshot names.
- Retry name collisions at most 100 times, advancing the name by one nanosecond.
- Retain legacy second-precision snapshots and prune by parsed timestamp.

## Capabilities

### New Capabilities

- `command-interface`: collision-safe post-collection archive snapshots.

### Modified Capabilities

None.

## Impact

Backup filenames change. Existing snapshots remain eligible for retention.
Archive contents, CLI flags, and runtime dependencies do not change.
See [ADR 0014](../../../docs/adr/0014-unique-archive-backups.md).
