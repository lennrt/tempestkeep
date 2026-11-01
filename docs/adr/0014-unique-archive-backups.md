# ADR 0014: Unique archive backup names

## Status

Proposed.

## Context

Post-collection snapshots use second-precision names and atomic no-overwrite
publication. A second backup in the same second fails because its name exists.

## Decision

Use fixed UTC nanosecond-precision names. On collision, advance the candidate
instant by one nanosecond, up to 100 attempts. Keep atomic hard-link publication,
private permissions, temporary-file cleanup, and bounded directory reads.
Recognize both old and new names for retention. Compare parsed timestamps and
use filename order only when timestamps tie.

## Consequences

New backup filenames include a fractional second. Legacy backups still rotate.
An occupied candidate cannot be overwritten. Exhausted retries return an error.
Clock rollback retains the existing timestamp-based retention limitation.
No archive schema, public Go API, or runtime dependency changes.
