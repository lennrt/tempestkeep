## Implementation

- [x] Add status command and read-only report with regression tests.
- [x] Add command and JSON usage documentation.

## Verification

- [x] Run focused CLI tests, full make verify, and strict specification validation.
- [x] Obtain independent integrated review and record remaining limitations.
- [x] Reproduce UTF-16 metadata limit failures, repair the bounded reader, and
  pass encoding, NULL, NUL, and storage-class regressions with finite timeouts.
- [x] Reproduce size-guard bypass through a nondeterministic legacy view, then
  materialize one value and pass a deterministic single-evaluation regression.

Independent review reproduced a 16 MiB metadata allocation. A shared reader fix
bounds metadata in SQL and preserves validation error classification. Repeating
the experiment used about 7 KiB of Go allocation. Boundary, NUL, legacy-table,
and cancellation tests passed. Full store/collector tests passed after repair.
Status mutation checks killed 25 of 28 variants. Three survivors are equivalent
under shared coverage-extrema validity and RFC3339 subsecond formatting. The
tool counts any nonzero test exit or timeout as killed; no separate compilation
failure count is available. Measured status-function CRAP scores are below 10.

Final integrated `make verify` passed on Go 1.27.3 on 2026-11-01. The pinned patch
also passed the vulnerability scan after Go 1.27.2 reported GO-2026-6809.

A later adversarial review reproduced both rejected valid ASCII values and
accepted oversized CJK values in UTF-16 archives. The repaired SQL bounds the
encoded projection at 8,192 bytes. Both readers then enforce the existing
4,096-byte UTF-8 limit. Tests cover UTF-8, UTF-16le, UTF-16be, NULL, NUL, and
non-TEXT storage classes. The affected CLI, store, API, and collector packages
passed ordinary and race tests with `-count=1 -timeout=3m` on Go 1.27.3. Strict
OpenSpec validation passed all 19 items. Integrated checks remain recorded by
the coordinating review after all concurrent edits finish.

A further synthetic legacy view returned a 1 MiB string after its size guard
had evaluated a different, small value. Materializing one matching value keeps
the guard and projection consistent. A deterministic test-only scalar failed
before the fix with three evaluations and passed afterward with one evaluation
per read. SQL-engine schema evaluation remains outside the Go-string bound.
