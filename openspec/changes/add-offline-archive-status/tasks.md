## Implementation

- [x] Add status command and read-only report with regression tests.
- [x] Add command and JSON usage documentation.

## Verification

- [ ] Run focused CLI tests, full make verify, and strict specification validation.
- [x] Obtain independent integrated review and record remaining limitations.

Independent review reproduced a 16 MiB metadata allocation. A shared reader fix
bounds metadata in SQL and preserves validation error classification. Repeating
the experiment used about 7 KiB of Go allocation. Boundary, NUL, legacy-table,
and cancellation tests passed. Full store/collector tests passed after repair.
Status mutation checks killed 25 of 28 variants. Three survivors are equivalent
under shared coverage-extrema validity and RFC3339 subsecond formatting. The
tool counts any nonzero test exit or timeout as killed; no separate compilation
failure count is available. Measured status-function CRAP scores are below 10.
