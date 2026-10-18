# Design

## Recovery boundaries

Use the existing collector for explicit CLI ranges. Require both date flags,
include each local calendar day, and limit a request to 366 calendar days.
Reject future end dates and explicit seed-floor conflicts before discovery or
archive creation. A configured environment seed floor applies only to ordinary
collection. Explicit repair does not change seed or MCP sync metadata.
Checkpoint and backup rules remain unchanged. On interruption or failure, keep
committed chunks and tell the caller to repeat the same range arguments.

MCP `before_epoch` is an optional positive integer, exclusive like `end`.
Reject simultaneous `end` and `before_epoch`, unsupported epochs, and empty
ranges before external work. Both forms select explicit backfill and leave
shared continuation metadata unchanged. The returned `next_before` can be
passed directly as `before_epoch`; callers repeat their `start` and `max_days`.
Undated calls retain their existing persistent-cursor behavior.

## Sensor coverage

Keep the existing numeric `rain_in` and `lightning_strikes` totals for source
compatibility. Add `rain_readings` and `lightning_readings` using SQL COUNT of
each sensor column. A zero count means unknown, a positive count with zero
sum means measured zero, and a count below `obs` means partial coverage.
The app derives display rows without changing the tool result, suppresses
unknown event totals, and warns when coverage is incomplete. Older results
without counts retain their existing display behavior.

## Alternatives and limits

Nullable totals would break exported Go field types; additive coverage also
expresses partial buckets. Automatic gap repair would add policy and repeated
external work, so users choose explicit ranges. Recovery cannot restore data
that the upstream service does not provide. Sensor counts describe samples,
not continuous uptime. Calendar ranges and MCP 24-hour budgets are distinct.

## Verification

Use synthetic archives and API servers. Exercise DST, exact continuation,
replay, cancellation, partial failure, invalid input before I/O, and metadata
preservation. Test missing, zero, partial, and nonzero sensor buckets in Go,
MCP JSON, and the sandboxed browser. Run repository verification and inspect
the rebuilt frontend before publishing the consolidated PR.
