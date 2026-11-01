## Implementation
- [x] Add failing synthetic JSON coverage regressions (TestHistoricalRainCoverage and
  TestHistoricalRainNormalsDenominator reproduced old zero/missing-count/false-denominator
  behavior).
- [x] Preserve nullable totals/counts and rain-specific normal denominator (Go1.27.2 store
  package tests pass).
- [x] Adapt CLI/MCP and run relevant Go tests and strict OpenSpec validation (focused tests and
  race pass; actual MCP schema and ordinary/structured equivalence verified).
- [x] Show missing/partial coverage and rain-normal contributors in the app;
  three new browser cases and the integrated 63-case browser suite passed.
- [x] Independently review the integrated change. Correct the this-day rain cell
  partial marker; the original reproduction and terminal width checks passed.
- [x] Preserve gaps in the terminal day rain sparkline without losing reported zero or
  partial totals. The renderer regression failed against the original renderer with a Go
  overlay, then passed with focused explorer and terminal checks and their race run on Go
  1.27.3. Strict validation of this change passed.
