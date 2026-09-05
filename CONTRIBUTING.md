# Contributing

Keep changes small enough to review. State every public, wire, storage,
security, or configuration effect.

## Contribution process

Use [GitHub Issues](https://github.com/lennrt/tempestkeep/issues) to discuss bugs
and proposed enhancements. English reports, documentation, and code-review
comments are welcome. Follow [SUPPORT.md](SUPPORT.md) for public reports and
[SECURITY.md](SECURITY.md) for private vulnerability reports.

Fork the repository, create a focused branch, and open a pull request against
`main`. Complete the pull request template with the problem, compatibility
impact, and verification results. Maintainers review the proposal, request
changes when needed, and merge accepted work. Keep interim commits available for
review; do not submit only a final source archive.

## Prerequisites

- Go 1.27.0.
- A C toolchain for race tests.
- Network access to the Go module proxy for the first tool download.

Do not use a live token for normal tests. Do not add a token, `.env`, archive,
backup, export, station identifier, coordinate, serial number, or captured API
response to Git.

## Required checks

Run from the repository root:

```sh
go mod download
make verify
```

`make verify` checks the exact Go version, modules, formatting, imports, module
tidiness, documentation and local links, vet, ordinary tests, race tests,
focused fuzz tests, lint, generated public API evidence, GitHub Actions syntax,
vulnerabilities, licenses, SBOM generation, secrets, and pure-Go builds for the
host and Linux ARM64.

Run a focused check when the full suite is not needed:

```sh
make test
make integration
make e2e
make lint
```

Do not turn a missing prerequisite into a passing skip in required CI.

## Design rules

- Store observations in SI units. Convert only at a display or documented wire
  boundary.
- Keep one device per archive.
- Keep analytical reads on `store.Store`, which is read-only.
- Keep collection writes on `store.Writer`. Observation writes are append-only
  and replay-safe.
- Put `context.Context` first on blocking public operations. Do not retain it.
- State whether a constructor performs I/O. Configuration constructors must not
  perform I/O.
- Copy retained caller-owned slices, maps, and byte buffers.
- Bound input and output before allocation or external work.
- Preserve errors that callers classify with `errors.Is` or `errors.As`.
- Keep `Close` behavior explicit and idempotent where promised.
- Keep the production dependency graph compatible with `CGO_ENABLED=0`.
- Keep MCP stdout for JSON-RPC only.

Add an ADR before changing a public API, JSON field, archive schema, security
boundary, configuration meaning, or release behavior. Update tests and
`docs/public-api.txt` with `make api-update` when an approved public API change is
intentional.

## Tests

Every major new feature must include automated tests for its behavior. A bug fix
must include a regression test when the failure can be reproduced safely.
Explain any exception in the pull request. The TUI changes in version 0.2.0,
for example, include frame-bound, scroll, retry, and stale-response regressions.

Each test must have a finite timeout through the test command or its context.
Avoid fixed readiness sleeps. Use events, polling with a deadline, an injected
clock, or a protocol canary. Clean up every resource and report cleanup errors.

Add regression coverage for malformed input, bounds, cancellation, replay,
concurrency, partial failure, restart, and cleanup when those cases apply. See
`docs/testing/properties.md`.

Run `go test ./... -count=1 -timeout=5m` for the standard Go test invocation.
Use `make cover` to inspect statement coverage. Coverage does not establish
branch coverage or prove that every important failure has been tested.

## Security review

Apply [the secure-development guide](docs/secure-development.md) and the
[threat model](docs/threat-model.md) when changing a trust boundary. Use Go's
standard cryptographic libraries rather than implementing cryptography.
Keep `go vet`, the enabled golangci-lint analyzers, CodeQL, race checks, and
fuzzing active. Fix findings before merging, or document a specific false
positive beside a narrowly scoped suppression. Do not disable a check to make
an uninvestigated finding pass. Follow [SECURITY.md](SECURITY.md) for confirmed
static-analysis and dynamic-analysis vulnerabilities.

## Documentation

Use short, direct sentences. Put a condition before the action. Use one term for
one meaning. State prerequisites, exact commands, limits, results, ownership, and
failure behavior. Do not add marketing claims or readiness claims without
recorded evidence.

## Publication

Do not publish a release or plugin from an unreviewed change. The release
workflow builds a local snapshot. Publication needs a separate maintainer
decision and release evidence.
