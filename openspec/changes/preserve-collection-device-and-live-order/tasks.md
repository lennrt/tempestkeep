## Implementation and verification

- [x] Add and execute failing synthetic regression tests before implementation.
- [x] Implement bounded archive-aware selection and timestamp ordering.
- [x] Run focused and full affected Go packages and race checks with finite timeouts.
- [x] Run strict OpenSpec validation. All 19 items passed during
  the integrated adversarial review.
- [x] Reproduce and fix identity selection for UTF-16 archives and
  case-insensitive legacy metadata table names; pass regressions and affected
  package ordinary and race tests with `-count=1 -timeout=3m` on Go 1.27.3.
