# Homebrew tap

TempestKeep targets a project-maintained Homebrew tap. It does not target
`Homebrew/homebrew-core`.

## User contract

The tap repository is `lennrt/homebrew-tap`. Its formula is `tempestkeep`. A
published formula installs one executable and no service:

```sh
brew install lennrt/tap/tempestkeep
tempestkeep version
tempestkeep help
tempestkeep help mcp
```

Do not install `tempest` or `tempest-mcp` aliases. They can shadow unrelated
commands.

## Release prerequisites

Before updating the tap:

1. Obtain explicit publication approval.
2. Select a clean, qualified source revision.
3. Publish one signed, immutable stable tag.
4. Make the source archive available without credentials.
5. Calculate SHA-256 from the final source archive.
6. Confirm that the release reports its version as `tempestkeep <version>`.

Do not move a published tag. Publish a new patch version after a release error.

## Formula contract

The formula must:

- build `./cmd/tempestkeep` with Homebrew's Go build dependency;
- set `CGO_ENABLED=0` and `GOTOOLCHAIN=local`;
- inject `github.com/lennrt/tempestkeep/internal/version.version`;
- fetch checksum-verified Go modules before the offline build;
- build with `-mod=readonly` and `GOPROXY=off`;
- install only `bin/tempestkeep`;
- use `license "MIT"`;
- omit a service and compatibility aliases; and
- run a deterministic archive operation without a token or network access.

The formula must not run `make build`. A source archive has no `.git` directory,
so the Makefile version fallback would otherwise report `dev`.

## Local verification

Run against the reviewed formula in the local tap:

```sh
brew style lennrt/tap/tempestkeep
brew audit --strict --online lennrt/tap/tempestkeep
HOMEBREW_NO_INSTALL_FROM_API=1 \
  brew install --build-from-source lennrt/tap/tempestkeep
brew test lennrt/tap/tempestkeep
brew linkage --test --strict lennrt/tap/tempestkeep
brew livecheck --debug lennrt/tap/tempestkeep
tempestkeep version
tempestkeep help mcp
```

Then uninstall and untap it. Confirm that the public one-command path works from
an untapped state:

```sh
brew uninstall --force tempestkeep
brew untap lennrt/tap
brew install lennrt/tap/tempestkeep
brew test lennrt/tap/tempestkeep
```

Use current Homebrew-generated tap workflows as the starting point. Pin actions
by full commit SHA, grant minimum permissions, cancel obsolete runs, and use no
scheduled workflow. Publish bottles only from an explicitly reviewed commit.

Homebrew tests and workflows must not receive `TEMPEST_TOKEN`, a station ID, an
archive, or captured API data.
