## Implementation

- [x] Add opt-in export metadata and a streaming store iterator; verify focused
  export and store regressions, including unchanged default output.
- [x] Preserve compatible table controls and focus; verify browser regressions
  for refresh, changed schemas, new inputs, failure, and stale responses.
- [x] Align active version metadata and documentation at 0.4.0; verify the
  development binary and embedded app version without publishing a release.
- [x] Document metadata units, missing values, source bounds, solar arithmetic,
  refresh scope, and compatibility; update generated public API evidence.

## Verification and Delivery

- [x] Challenge the implementation and tests through independent review.
- [x] Run `make verify` and strict specification checks against the final source.
- [x] Run app build, bundle, browser, and screenshot checks; inspect rendered UI.
## Recorded Evidence

- `make verify` passed on Go 1.27.2, macOS ARM64, including ordinary and race
  tests, four fuzz targets, lint, security checks, builds, and the MCP demo.
- Full store and CLI package tests passed. Focused race tests also passed for
  the metadata iterator, legacy reads, exports, range rules, and SQL compilation.
- The original exporter rejected `--metadata` in all four format/unit workflow
  cases. The updated tests reproduce 0.006 MJ/m² from one exported sample.
- `npm run app:test` passed 40 browser tests. The old bundle failed the new
  retained-search regression. The fixed test also checks that fresh row values
  appear with the retained sort and page.
- App build, bundle check, and screenshot capture passed. Desktop and 390-pixel
  filtered/sorted refresh views were inspected independently.
- Strict specification validation passed all nine items. Documentation checks
  passed all 75 Markdown files. Public API evidence and diff checks passed.
- Version tests, the Makefile build, and a build without VCS module stamping
  confirm `v0.4.0-dev`. Existing module and linker version precedence is preserved.
- Two independent reviewers found no remaining correctness or test blockers.

Publication and hosted CI results are recorded in PR #9. Delivery uses the
existing branch and keeps the pull request open and unmerged.
