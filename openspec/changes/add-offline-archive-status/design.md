## Contract

Validate `--format text|json` before dotenv/archive work. Resolve the database
using existing CLI precedence. Store.Open must not create a missing archive.
Use existing Coverage, Gaps, and Meta readers. Report gaps strictly greater than
3600 seconds, largest first, limited to ten. This is not a whole-range audit.

Use null timestamps and age for an empty archive and an empty gaps array. Fresh
means newest age is nonnegative and less than 3600 seconds. The existing saved
completion marker accepts absent/0/1; other values retain ErrInvalidCheckpoint.
Propagate query, cancellation, close, and output errors. No token is needed.

Metadata limits use UTF-8 bytes, matching Writer.SetMeta. SQLite BLOB casts use
the database encoding, so SQL allows up to twice that byte limit before Go
allocation. This admits valid UTF-16 values and bounds the converted UTF-8
string at three times the limit. The reader then enforces the original limit.
Materialize one matching value before applying the guard and projection, so a
legacy view cannot reevaluate a nondeterministic expression between them. This
bounds the driver's Go string, not SQLite's internal schema-evaluation memory.

## Verification and fallback

Run CLI tests for seeded/empty archives, JSON/text, strict gap threshold, invalid
format before I/O, missing-file preservation, corrupt metadata, future/stale
freshness, output errors, and read-only behavior. Inject time into the report
builder for deterministic comparisons. Run full make verify and strict specs.
The smaller fallback omits freshness while retaining coverage, gaps, and progress.
