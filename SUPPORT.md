# Support

Use [GitHub Issues][issues] for reproducible bugs and focused feature requests.
Search existing reports before opening a new one. Issues and their responses
are public, searchable, and addressable by URL. English reports are welcome.
Use [the private security process](SECURITY.md) for a vulnerability or secret leak.
Do not put sensitive station data in either channel.

## Before opening an issue

1. Read the [quickstart](README.md) and the relevant command guide in
   [docs/README.md](docs/README.md).
2. Read the [troubleshooting guide](docs/troubleshooting.md) for known recovery steps.
3. Run `tempestkeep help <command>` or inspect the MCP tool description.
4. Reproduce the problem with the default branch and Go 1.27.0 when practical.
5. Run the smallest relevant check from [CONTRIBUTING.md](CONTRIBUTING.md).

## Include

- The output of `tempestkeep version` and the source revision from `git rev-parse HEAD`.
- The operating system and architecture.
- The output of `go version` when building from source.
- The exact command or MCP operation, with credentials and identifiers removed.
- The expected result, observed result, and exit status.
- The MCP client's name and version, if the issue involves MCP.
- The process timezone and date range, if the issue involves calendar results.
- A minimal synthetic reproducer when possible.

Do not include tokens, `.env` files, or token-bearing URLs. Also omit station
names, coordinates, identifiers, serial numbers, archives, exports, raw API
responses, and private filesystem paths. Replace sensitive command
arguments with placeholders. Keep exact error wording after removing private
values, because the wording helps identify the failing operation.

## Scope

The maintainer triages public reports at least weekly and aims to acknowledge
bugs and enhancement requests within 14 days. An acknowledgment can request
more information, explain a limitation, or decline a proposal. Review the last
12 months of reports when updating the OpenSSF assessment. A future response
policy is not evidence of past responses.

Support is best effort. Security reports follow [SECURITY.md](SECURITY.md).
WeatherFlow availability, account permissions, host security, and MCP
client behavior are outside this project's control.

Bug fixes target the default branch. Support for a tagged version must be stated
in that version's release notes.

[issues]: https://github.com/lennrt/tempestkeep/issues
