# Tasks

- [x] Reproduce same-clock backup failure with a deterministic regression.
- [x] Implement bounded collision retries and timestamp-based legacy retention.
- [x] Pass focused backup tests and their race run with finite timeouts.
- [x] Pass strict OpenSpec validation on the integrated changes.

## Verification evidence

The three new regressions failed before the fix: repeated same-clock snapshots
collided, mixed-format retention removed the wrong file, and a fully occupied
candidate range did not fail as specified.

`go test ./cmd/tempestkeep -run 'Test(Snapshot|CopyFile|PruneBackups|PruneMixedBackup)' -count=1
-timeout=60s`
passed. The same selection with `-race` and `-timeout=90s` passed, including the
strengthened checks for prior snapshot bytes and retention after repeated calls.
Strict `npm run spec:check` passed 14 items. These tests use synthetic local
files; they do not claim full integrated repository verification.
