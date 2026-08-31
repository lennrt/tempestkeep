# Contributing

Keep changes small enough to review. State every public, wire, storage,
security, or configuration effect.

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

Each test must have a finite timeout through the test command or its context.
Avoid fixed readiness sleeps. Use events, polling with a deadline, an injected
clock, or a protocol canary. Clean up every resource and report cleanup errors.

Add regression coverage for malformed input, bounds, cancellation, replay,
concurrency, partial failure, restart, and cleanup when those cases apply. See
`docs/testing/properties.md`.

## Documentation

Use short, direct sentences. Put a condition before the action. Use one term for
one meaning. State prerequisites, exact commands, limits, results, ownership, and
failure behavior. Do not add marketing claims or readiness claims without
recorded evidence.

## Publication

Do not publish a release or plugin from an unreviewed change. The release
workflow builds a local snapshot. Publication needs a separate maintainer
decision and release evidence.
