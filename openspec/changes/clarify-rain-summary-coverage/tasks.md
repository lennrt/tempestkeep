## Implementation

- [x] Add nullable store totals and counts with old-code regression evidence,
  range/DST/missing/zero/partial/error tests and unchanged spell behavior.
- [x] Update CLI and MCP text/JSON/schema with workflow regressions.
- [x] Update the scalar rain app with legacy/refresh/coverage/browser checks.
- [x] Publish a feature-focused docs overview, query guidance, ADR, release notes,
  public API evidence, and test properties.

## Verification

- [x] Independently review the specification, implementation, and test strength.
- [x] Run make verify, strict specifications, app build/check/test/screenshots,
  inspect desktop/narrow views, and record exact evidence.

Deliver logical commits to the existing PR #9 and inspect hosted checks.
Keep the PR open and unmerged.

## Evidence

- Original store serialization and CLI command regressions failed on missing
  totals encoded as zero and absent coverage fields. The old app failed the
  stable rainfall-headline regression.
- Full make verify passed on Go 1.27.2, macOS ARM64, including ordinary and race
  tests, four fuzz targets, lint, API evidence, pure-Go builds, security checks,
  and the synthetic MCP demo.
- Actual MCP discovery requires total and count fields and permits number/null.
  Empty, missing, measured-zero, partial-zero, partial-positive, and reported
  positive fixtures pass in both structured and ordinary tool content.
- App build, bundle check, 55 browser tests, and screenshot generation passed.
  Desktop, 390-pixel missing-rain, and partial-rain views were inspected.
- Strict specifications passed all eleven items; documentation checks passed
  89 Markdown files. Independent code and contract reviews found no blockers.
- Tests use synthetic archives and hosts. No live account or named third-party
  client was tested. Version remains 0.4.0-dev; hosted evidence belongs in PR #9.
