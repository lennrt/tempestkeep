# ADR 0016: Offline archive status command

Status: proposed

## Context

The CLI repairs selected archive dates, but only MCP exposes the largest gaps.
The stats command runs climate analyses and does not report collection progress.
Terminal users and scripts need a short read-only inspection before a repair.

## Decision

Add `tempestkeep status --db PATH --format text|json`. Reuse Store.Coverage,
Store.Gaps, and Store.Meta. Open only an existing archive and perform no network
requests. Report stored observation count, first/last timestamps, newest-row age,
freshness, saved backfill completion, and up to ten largest gaps over one hour.

Use a fixed one-hour freshness threshold. A future observation is not fresh.
JSON encodes absent timestamps and age as null, and absent gaps as an empty array.
The existing completion marker accepts only absent, 0, or 1. Reject other values
with the existing invalid-checkpoint error. Validate format before configuration.

## Consequences

This adds a command and a JSON shape without changing a public Go API or archive
schema. Output does not expose the archive path or credentials. Stored sample
counts and internal gaps do not prove continuous coverage or sensor availability.
The fixed gap limit and threshold appear in the output. First and last gaps
outside the stored observation range cannot be inferred.

Independent review found that an externally modified metadata value could exceed
the writer's existing 4,096-byte limit. Both metadata readers now bound the SQL
projection and enforce that same UTF-8 byte limit after conversion. SQLite BLOB
casts measure the database encoding. The SQL bound is 8,192 encoded bytes so
valid values in UTF-16 archives remain readable. The projected UTF-8 string
contains at most 12,288 bytes. The reader rejects values over 4,096 UTF-8 bytes.
Materialize one matching metadata value before checking its size. This keeps
legacy views with nondeterministic expressions from changing the value between
the guard and projection. The limit bounds Go strings returned by the driver;
it does not bound SQLite's internal work to evaluate an external schema.
Null and NUL-containing values also return ErrInvalidArchive. Ordinary invalid
completion markers still return ErrInvalidCheckpoint. Store.Meta still supports
missing legacy metadata tables. This is an intentional read-validation change without an API shape
change, schema migration, or new limit.
