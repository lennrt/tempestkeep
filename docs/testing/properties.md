# Correctness properties

This catalog converts repository guarantees into test targets. A passing test is
evidence for the tested case. It is not proof of the general property.

All commands require Go 1.27.0. Tests have finite timeouts. Focused fuzz tests
store any failing input in the Go fuzz corpus so the failure can be replayed.

| ID | Property | Faults and boundaries | Current verification |
|---|---|---|---|
| P-01 | One archive accepts observations from only one device. | Two writers race to bind different devices; legacy rows disagree with metadata. | `TestConcurrentWritersCannotClaimDifferentDevices`, `TestWriterRejectsSecondDevice`, `TestOpenRejectsMixedDeviceArchive` |
| P-02 | Replaying the same observation does not add a duplicate or alter the first row. | Duplicate chunks, repeated collection, and restart from an earlier cursor. | `TestWriterInsertIdempotentAndReadParity`, `TestBackfillRangeChunksAndIsIdempotent` |
| P-03 | An invalid observation batch changes no archive state. | Invalid field after valid fields; oversized batch; invalid device. | `TestInsertObsRejectsInvalidBatchAtomically` and model validation tests |
| P-04 | A committed collection chunk remains visible after a later failure. | Fetch failure, progress-sink failure, cancellation, and restart. | `TestBackfillRangeResumesAfterError`, `TestProgressFailureReportsCommittedResumePoint`, `TestCollectSeedInterruptedResumes` |
| P-05 | A collection window is bounded, ordered, contiguous, and does not overlap its neighbor. | Minimum window, five-day limit, operation budget, and epoch boundaries. | Collector chunk tests and API window tests |
| P-06 | One `Backfiller` runs at most one operation at a time. | Concurrent `Sync`, forward fill, and backward fill calls. | `TestBackfillerRejectsConcurrentOperations`; `make race` |
| P-07 | Cancellation stops a request, retry wait, throttle wait, or query by its deadline. | Dependency outage and long configured waits. | `TestRetryStopsOnCancellation`, `TestBackfillerCancellationStopsThrottle`, context tests; `make race` |
| P-08 | Retry attempts and waits stay within the configured bounds. | HTTP 429, HTTP 5xx, transport failure, `Retry-After`, and cancellation. | API retry tests |
| P-09 | Diagnostic errors do not contain a token, URL, response body, station ID, device ID, query, or local path. | Transport errors, malformed responses, HTTP failures, configuration I/O, store I/O, and cleanup errors. | API, configuration, and store redaction tests; diagnostic review |
| P-10 | External input is rejected before an unbounded allocation or operation. | Oversized HTTP body, response counts, dotenv file, SQL, rows, columns, metadata, observation batch, backup directory, and pagination-like limits. | Limit-specific unit tests, fuzz tests, and lint review |
| P-11 | The read handle cannot mutate the archive. | Direct DML, CTE-prefixed DML, multiple statements, and validator bypass attempts. | `TestQueryReadOnly`, `TestQueryRejectsWrites`, and SQLite `query_only` |
| P-12 | Caller-owned retained data is copied. | Mutation of an `obs_st` row or station device slice after a call. | `TestDeviceObsFromRowCopiesAndValidates`, `TestPickTempestDevice` |
| P-13 | `Close` is safe to repeat. The zero value behaves as closed. | Nil receiver, repeated calls, and use after close. | Store and writer close tests; `make race` |
| P-14 | A backup is complete, private, and never overwrites an existing snapshot. | Name collision, source or directory symlink, copy failure, permissive directory, and unrelated files in the backup directory. | `collect_backup_test.go` |
| P-15 | MCP stdout contains protocol traffic only. Diagnostics contain no secret or local archive path. | Startup failures, live failures, write failures, and timezone warnings. | MCP protocol tests, stderr review, and secret scan |
| P-16 | Generated public API evidence matches the source. | Added, removed, or changed exported declarations and JSON tags. | `make generated` |
| P-17 | The production graph builds without cgo on the host and Linux ARM64. | Native dependency introduction and architecture-specific code. | `make build-pure build-arm64` |
| P-18 | A live dashboard cancels dependent work after a required fetch fails and reports optional forecast failure. | Missing observations, stalled forecast, configured archive failure, and concurrent station resolution. | `TestNowLoadCancelsForecastAfterObservationFailure`, `TestNowLoadReportsOptionalForecastFailure`, `TestResolveNowConfigRejectsUnavailableConfiguredArchive`; `make race` |

## Test commands

```sh
make test
make integration
make e2e
make race
make fuzz
make generated
```

Use `go test -run <name> -count=1` to replay a named regression. Go prints and
stores the input for a fuzz failure. Commit a minimized failing input only after
checking that it contains no token, payload, personal data, or raw identifier.

## Live smoke test

A live smoke test is optional locally and is not a required CI check. Load a
short-lived token into the process environment from a secret manager. Then run:

```sh
make live-smoke
```

The script discards API output. It creates an owner-only temporary archive,
checks one bounded history range and offline replay, and removes the archive on
exit. Do not save station names, coordinates, serial numbers, device IDs, or
token-bearing URLs as evidence.

The live smoke test does not establish durability, performance, service
availability, or production readiness.
