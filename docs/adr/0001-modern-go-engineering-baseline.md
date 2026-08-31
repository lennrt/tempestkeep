# ADR 0001: Modern Go engineering baseline

- Status: Accepted
- Date: 2026-08-29
- Owner: Repository maintainer
- Scope: Public Go API, configuration, storage, wire formats, CI, and releases

## Context

The initial repository state contains a README and MIT license. This decision
establishes the first Go implementation and compatibility record.

The implementation separates live API access, collection, storage, and command
front ends. It does not consistently pin the Go patch version or tools. Several
public constructors read ambient state or perform blocking work without a
context. Some public inputs and returned collections are not bounded.

## Decision

Use Go 1.27.0, the stable release listed by `https://go.dev/dl/?mode=json` on
2026-08-29. Pin that exact patch in the module, CI, release jobs, and operator
documentation.

Establish the package surface as the compatibility baseline:

- Blocking operations take `context.Context` first.
- Constructors that only configure values have no network, filesystem,
  goroutine, migration, or environment side effects.
- Filesystem-opening APIs state their side effects in the name and contract.
- Retained dependencies and values have explicit ownership rules.
- Public inputs, caches, batches, queries, results, retries, and diagnostics
  have documented limits.
- Public failures use sentinel or typed errors compatible with `errors.Is` and
  `errors.As`.
- Close operations are safe to call more than once.
- The archive remains one-device, append-only for observations, and read-only
  for analytical queries.
- The production dependency graph remains pure Go with `CGO_ENABLED=0`.
- Existing JSON field names remain unchanged during this baseline. Any later
  serialization change requires a separate ADR and golden-wire update.

Pin GitHub Actions by full commit SHA and grant each job minimum permissions.
Required CI runs on pushes to `main`, pull requests, and manual dispatch. Do not
add schedules. Release qualification is manual and builds snapshots only.
GoReleaser publication is disabled.

## Compatibility

These API refinements are breaking source changes relative to the superseded
local design. The CLI and MCP server also remove token flags. They accept
`TEMPEST_TOKEN` from a private environment or `.env` file. This prevents tokens
from entering shell history and process diagnostics.

The API snapshot and external consumer fixture establish this baseline. Later
incompatible public changes require an explicit compatibility decision and ADR.

The archive schema and existing JSON field names do not change in this ADR.

## Verification

Run the commands documented in `CONTRIBUTING.md` and CI. Required evidence
includes formatting, goimports, vet, unit and integration tests, race tests,
fuzz smoke tests, lint, API snapshot checks, generated-file checks, pure-Go
cross-builds, vulnerability and secret scans, dependency license checks, and
SBOM generation.

## Consequences

The minimum Go version becomes 1.27.0. Contributors and CI may download that
toolchain through Go's verified toolchain mechanism.

The stronger API contracts require call-site and test updates for this baseline.
CI gains additional security jobs and therefore uses more runner time. No
workflow in this decision may commit, push, publish, tag, or create a release.
