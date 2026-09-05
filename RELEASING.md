# Release process

Only the repository maintainer can approve a release. Qualification commands do
not publish, tag, push, or change repository visibility. GoReleaser publication
remains disabled until a separate reviewed change enables an approved release.

## Preconditions

- Obtain explicit release approval.
- Select one reviewed commit with a clean working tree.
- Require all configured GitHub checks to pass for that commit.
- Use Go 1.27.0.
- Confirm that no credential, station data, archive, export, personal path, or
  raw API response is present in tracked, untracked, ignored, or generated
  files.
- Resolve every compatibility, security, license, and publication finding or
  record a narrow owner-approved exception.

## Classify compatibility

For the first release, review `docs/public-api.txt` as the public package
baseline. For later releases, compare it with the previous tag.

Record an ADR for a public API, JSON, MCP, archive schema, configuration,
security-boundary, or release-process change. State whether the release changes
source, wire, storage, or operational compatibility.

Choose a semantic version. Do not reuse or move a published tag.

Use `vMAJOR.MINOR.PATCH` for release tags and the corresponding SemVer version
for artifacts. Development snapshots retain the exact source commit as their
unique identifier; `v0.2.0-dev` alone does not identify a source revision.

Update [CHANGELOG.md](CHANGELOG.md) with a human-readable account of changes,
compatibility effects, and upgrade instructions. Include every publicly known
run-time vulnerability fixed by the release that already has a CVE, GHSA, or
similar identifier. Include the identifier and mitigation or upgrade guidance.
When there are none, say so after reviewing the project's advisories.

## Qualify the source

Run from the repository root:

```sh
go version
make verify
make integration e2e
make release-check
```

The Go version must be exactly 1.27.0. The first three commands must pass. The
last command creates local snapshot archives in `dist/` without publishing them.

Inspect every archive. Confirm that it contains the `tempestkeep` binary,
`LICENSE`, `NOTICE.md`, `README.md`, and `docs/tempestkeep-hero.jpg`. Verify
`dist/checksums.txt` against the archives:

```sh
(
  cd dist
  shasum -a 256 -c checksums.txt
)
```

Generate the CycloneDX SBOM with `make sbom` and retain its digest with the
qualification evidence.

Record:

- source revision and proposed tag;
- Go, GoReleaser, and tool versions;
- target operating systems and architectures;
- archive and SBOM digests;
- exact commands and results;
- compatibility and wire-format classification;
- dependency, vulnerability, license, and secret-scan results; and
- remaining limitations and nonclaims.

Deliver source, binaries, and checksum files through HTTPS. Do not use a hash
downloaded over unauthenticated HTTP to verify an artifact. Retain the checksums
and signature-verification instructions with any approved release.

Remove `dist/` with `make clean` after review unless the maintainer explicitly
retains it as local evidence.

Use [docs/homebrew.md](docs/homebrew.md) to qualify a project-tap formula from
the final public source archive. Tap publication is a separate approved change.

## Publish

Publication requires a separate maintainer-approved procedure. It must create a
signed, immutable tag from the qualified commit and attach the reviewed
archives, checksums, SBOM, release notes, and required notices. Do not publish
from an unclean tree or rebuild artifacts from a different commit.

## Correct a release

Do not delete, move, or reuse a published tag. If an artifact or statement is
wrong, document the problem and issue a new patch release. Use the private
security-advisory process when the correction concerns a vulnerability or
credential exposure.
