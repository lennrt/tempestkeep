# Design

## Context

The [proposal](proposal.md) lists the affected behavior. TempestKeep already has
separate read and write database handles, bounded HTTP requests, and typed MCP
tools. Preserve those boundaries and fix failures at the layer that owns them.

## Decisions

Read the SQLite checkpoint result row. An SQL execution without an error does
not prove that SQLite copied every WAL frame. Refuse the backup when the result
reports a busy reader or incomplete work. Keep the diagnostic free of file paths.

Validate response envelopes before treating their contents as observations.
Reject invalid JSON before inserting it into the cache. Saturate retry hints
before duration multiplication. Count series buckets from their aligned indices.
These checks belong in the API client and store, where every caller benefits.

Keep missing readings distinct from measured zeroes throughout analytics and
display. Use reported intervals for solar energy estimates and known headings
for directional wind sectors. Preserve public fields and record the corrected
semantics in [ADR 0005](../../../docs/adr/0005-reliability-and-specifications.md).

Use calendar arithmetic for local dates. Keep independent sensor readings visible
when temperature is absent. Validate a complete generated dotenv file before
replacing the old one, and quote generated PowerShell commands as literal paths.

Keep MCP date parsing before network work. Resolve explicit station IDs against
the accessible stations before labeling forecast output. Retain existing wire
fields and error categories. Reject ignored CLI arguments before command I/O.

Treat a bounded backfill call as part of a longer operation. Its work limit must
remain a hard cap. Persist progress only after successful chunk commits. Keep
the history-exhaustion decision consistent across separate calls and restarts.
Existing archives without the new private progress metadata start conservatively.

Keep the specification CLI in development dependencies. Pin its package and
transitive dependency lockfile. Run specification checks in their own CI job.
Keep the existing Go checks independent of Node.js.

Use SimpleEnglish Plain guidance for prose. Keep the names of tools, flags,
SQL columns, units, and errors unchanged. A shorter sentence must preserve its
original technical meaning. Run the example workflows with synthetic data.

## Risks and trade-offs

- Stricter validation rejects invocations and responses that previously appeared
  to succeed. Regression tests must show that each rejection protects a real boundary.
- A checkpoint can fail while another reader holds a snapshot. The operator can
  retry after that reader finishes. The command must not claim that a backup exists.
- Private backfill metadata needs conservative handling after older writers run.
  Restart and replay tests must cover absent or stale progress records.
- Specification validation checks document structure. It does not prove that
  implementation behavior matches a requirement. Keep tests linked to scenarios.

## Migration and rollback

No observation-table migration is required. Existing archives and clients retain
their public formats. Review the corrected input and work-limit behavior in the
ADR before merging. An older binary can ignore new private metadata, but it also
restores the defects that this change fixes.

Keep this change active during review. After acceptance, use OpenSpec archive to
apply its requirement additions to the baseline specifications.
