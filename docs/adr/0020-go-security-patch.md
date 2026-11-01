# ADR 0020: Go security patch

Status: proposed

## Context

The final vulnerability check found GO-2026-6809 in Go 1.27.2. Go 1.27.3
fixes this standard-library vulnerability. The Go release notes date the patch
to October 19, 2026. Earlier checks with 1.27.2 remain historical evidence.

Sources: [Go vulnerability record](https://pkg.go.dev/vuln/GO-2026-6809) and
[Go release notes](https://go.dev/doc/devel/release#go1.27.3).

## Decision

Require Go 1.27.3 in the module, Makefile guard, CI, CodeQL, demo, release
qualification, and active contributor guidance. Update the current OpenSpec
context and patch requirement. Preserve old ADRs, release notes, proposals,
and dated test records. Keep all module and npm dependency versions unchanged.

## Consequences

Contributors must select Go 1.27.3. The exact-version guard rejects 1.27.2.
Rebuild binaries with the fixed toolchain and repeat vulnerability checks.
This supporting security repair does not add an eleventh feature, publish a
release, or certify earlier binaries. Archive formats and application APIs
remain unchanged. Release approval remains a separate maintainer decision.
