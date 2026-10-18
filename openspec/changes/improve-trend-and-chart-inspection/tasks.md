# Tasks

- [x] 1. Correct the monthly trend fit. Pass
  `go test ./pkg/tempest/store -count=1 -timeout=5m` and its race run.
- [x] 2. Add accessible dense-chart inspection. Pass `npm run app:test`.
- [x] 3. Update interpretation guidance, screenshots, and release notes. Pass
  `make docs-check` and `npm run app:screenshots`, then inspect the images.
- [x] 4. Pass `npm run spec:check`, `npm run app:check`, and `make verify`.
- [x] 5. Independently review correctness, accessibility, and the integrated diff.

## Verification evidence

On 2026-10-18, the original estimator returned about 0.8973°F per decade for the
synthetic case that requires 36°F per decade. Exact warming and cooling
regressions now pass. Additional tests cover unmatched-month invariance,
uneven groups, equal monthly weights, missing temperatures, flat readings,
and numerical stability near the supported year limit.

Independent review found that CLI reports labeled zero slope as warming.
The label now reads flat, and `TestWriteStatsFlatTrend` protects it. The full
`make verify` run passed after this correction on Go 1.27.2, macOS ARM64.

All 32 Playwright checks passed. New chart tests exercise 2,000 points,
irregular spacing, missing and zero readings, source timezone offsets,
keyboard and touch selection, result changes, and selection/focus on resize.
The rebuilt bundle matches its sources. Desktop and 390-pixel screenshots
were inspected independently, with no remaining layout or interaction finding.
Strict OpenSpec validation passed all eight baseline and change items.

Browser evidence uses the official test host with synthetic data. It does not
certify a specific third-party client or establish climate significance.
