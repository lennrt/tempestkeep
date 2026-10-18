# Tasks

- [x] 1. Implement bounded CLI range repair. Pass the CLI regression command below.
- [x] 2. Implement exact MCP continuation. Pass
  `go test ./internal/mcpapp -run TestBackfill -count=1 -timeout=3m` and its race run.
- [x] 3. Add sensor counts. Pass
  `go test ./pkg/tempest/store ./internal/mcpapp -count=1 -timeout=5m`.
- [x] 4. Display missing and partial event coverage. Pass `npm run app:test`.
- [x] 5. Update user guidance, public API evidence, and release notes. Pass
  `make docs-check generated`.
- [x] 6. Pass `npm run spec:check` and `make verify` on the integrated changes.
- [x] 7. Independently review the diff and scenarios, and resolve findings.

## Verification evidence

The focused CLI command was:

```sh
go test ./cmd/tempestkeep -run 'Test(ExplicitCollectionRange|CollectRange)' -count=1 -timeout=60s
```

On 2026-10-18, `make verify` passed with Go 1.27.2 on macOS ARM64. This includes
all ordinary and race tests, four bounded fuzz targets, static analysis,
dependency/security checks, pure-Go host and Linux ARM64 builds, and the MCP demo.
Strict OpenSpec validation passed all eight baseline and change items.

The final app build and source check passed. All 32 Playwright tests passed.
The suite covers missing, measured-zero, partial, legacy, and SQL display cases.
Desktop and 390-pixel screenshots were generated and inspected.

The original CLI rejected the new range arguments. The original MCP server
rejected exact cursor input. The original series regression exposed unknown
sensor totals without counts. Each regression passes with the implementation.
Independent review also caught stale embedded/plugin continuation guidance,
which was corrected. A further regression confirms that coverage warnings and
coarser-resolution warnings remain visible together.

Tests use synthetic observations and local API fixtures. No live WeatherFlow
token or third-party MCP host certification was used. The Unix signal regression
is skipped on Windows because os.Interrupt cannot signal a subprocess there.
