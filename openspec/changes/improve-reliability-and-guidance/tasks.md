# Tasks

## 1. Archive and API reliability

- [x] 1.1 Reject incomplete checkpoints and pass the pinned-reader regression.
- [x] 1.2 Correct aligned series bounds and pass the limit-boundary regressions.
- [x] 1.3 Reject invalid API responses and pass cache, envelope, and retry tests.
- [x] 1.4 Preserve backfill progress and pass interruption and restart tests.
- [x] 1.5 Bound SQL values and columns inside SQLite and pass cancellation regressions.
- [x] 1.6 Correct solar estimates and wind sectors and pass analytical regressions.
- [x] 1.7 Preserve missing-sensor semantics and pass spell and comfort regressions.
- [x] 1.8 Fuzz nested and duplicate response collections without network access.

## 2. MCP and CLI behavior

- [x] 2.1 Correct calendar bounds and station labels with date and station regressions.
- [x] 2.2 Enforce backfill work caps and test repeated small-budget calls.
- [x] 2.3 Reject ignored CLI arguments and test validation before I/O.
- [x] 2.4 Match resources, prompts, and status guidance to configured capabilities.
- [x] 2.5 Preserve bounded forward sync and test restarts, failures, and late arrivals.
- [x] 2.6 Correct terminal output and setup boundaries and pass display regressions.
- [x] 2.7 Verify cross-source identity and pressure freshness with real MCP regressions.

## 3. Documentation and specifications

- [x] 3.1 Add executable examples and run them against synthetic data and real MCP.
- [x] 3.2 Apply SimpleEnglish guidance and run the documentation checker.
- [x] 3.3 Add OpenSpec baselines and deltas and pass strict validation.
- [x] 3.4 Pin development tooling and validate the CI workflow with actionlint.
- [x] 3.5 Correct fence and link parsing and pass documentation regressions.
- [x] 3.6 Verify the license-only dependency update and pass the native license scan.
- [x] 3.7 Rehearse archive in a scratch copy and strictly validate the resulting baselines.

## 4. Integration review

- [x] 4.1 Run available repository checks and record exact results and limitations.
- [x] 4.2 Review the complete diff independently and resolve confirmed findings.
- [x] 4.3 Prepare one draft PR or a ZIP with source, patches, and review evidence.

## Validation record (2026-09-30)

The full Go tests and race suite passed on native Windows with Go 1.27.0.
Vet, golangci-lint, formatting, imports, documentation, strict OpenSpec validation,
actionlint, module verification, tidy checks, and the public API snapshot passed.
Vulnerability and secret scans found no issues. The dependency license check and
synthetic CLI/MCP demo passed. An OpenSpec archive rehearsal in a scratch copy
produced five baseline specifications that passed strict validation.

Four network-free fuzz runs passed, with 69,594,584 executions in total:

| Target | Duration | Executions |
|---|---|---:|
| Endpoint responses | 60 minutes | 35,471,460 |
| Dotenv round trips | 5 minutes | 7,868,045 |
| Observation rows | 5 minutes | 22,765,418 |
| Scroll ranges | 5 minutes | 3,489,661 |

Pure-Go Windows, Linux ARM64, and macOS ARM64 builds passed. Linux and macOS
were cross-compiled; native runtime tests ran only on Windows. GNU Make was
unavailable, so the checks ran through their component commands.

Cloud CI has not run. GitHub has not registered the contribution fork's workflows,
and initializing its Actions setting awaits approval. No live WeatherFlow data,
production load tests, Antithesis runs, or formal correctness proof are included.
The [draft PR](https://github.com/lennrt/tempestkeep/pull/4) records the review scope
and limitations. Keep this proposal active until the change is accepted.
