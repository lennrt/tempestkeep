## Implementation

- [x] Add MCP selected-device configuration and lazy resolution with focused
  CLI, tool, retry, override, and archive identity regressions.
- [x] Add the bounded derived Devices table and exact identifier formatting;
  run browser regressions and inspect desktop/narrow views.
- [x] Correct pressure comparison selection and normalized output; verify
  predecessor/successor, ties, short history, small windows, and missing values.
- [x] Validate stats/export ranges before I/O; verify usage failures and DST bounds.
- [x] Update user guidance, release notes, API evidence when affected, and properties.

## Verification

- [x] Independent review of the specification, actual diff, and regression quality.
- [x] Run `make verify`, strict specifications, app bundle/browser/screenshot checks.

Publish logical commits through the existing PR #9 branch. Record hosted checks
in the PR description and keep it open and unmerged.

## Evidence

- Old code failed the selected-station, missing-device-table, wrong-direction
  pressure, short-history, and early report-validation regressions.
- Full `make verify` passed on Go 1.27.2, macOS ARM64, including ordinary and
  race tests, four fuzz targets, lint, API evidence, pure-Go builds, security
  checks, and the synthetic MCP demo.
- `npm run app:build`, `app:check`, `app:test` (47 tests), and `app:screenshots`
  passed. Desktop and 390-pixel Devices views were inspected independently.
- `npm run spec:check` passed all ten items. `make docs-check` passed 82 files.
- Independent reviewers found no remaining code blockers. Review added explicit
  browser integer-precision handling and its scenario. The subprocess test
  uses the existing two-second shutdown allowance for the race runtime.
- Version builds still report `v0.4.0-dev`. No live credentials or third-party
  client certification were used. Hosted checks are recorded in PR #9.
