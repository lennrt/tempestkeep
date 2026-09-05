# Support

Use [GitHub Issues][issues] for reproducible bugs and focused feature requests.
Search existing reports before opening a new one. Issues and their responses
are public, searchable, and addressable by URL. English reports are welcome.
Use [the private security process](SECURITY.md) for a vulnerability or secret leak.
Do not put sensitive station data in either channel.

## Before opening an issue

1. Read the [quickstart](README.md) and the relevant command guide in
   [docs/README.md](docs/README.md).
2. Run `tempestkeep help <command>` or inspect the MCP tool description.
3. Reproduce the problem with the default branch and Go 1.27.0 when practical.
4. Run the smallest relevant check from [CONTRIBUTING.md](CONTRIBUTING.md).

## Include

- the exact source revision from `git rev-parse HEAD`;
- the operating system and architecture;
- the output of `go version` when building from source;
- the exact command or MCP operation, with credentials and identifiers removed;
- the expected and observed result; and
- a minimal synthetic reproducer when possible.

Do not include tokens, `.env` files, station names, coordinates, station or
device identifiers, serial numbers, archives, exports, raw API responses, or
token-bearing URLs.

## Scope

The maintainer triages public reports at least weekly and aims to acknowledge
bugs and enhancement requests within 14 days. An acknowledgment may request
more information, explain a limitation, or decline a proposal. Review the last
12 months of reports when updating the OpenSSF assessment; a future response
policy is not evidence of past responses.

Support is best effort. Security reports follow [SECURITY.md](SECURITY.md).
WeatherFlow availability, account permissions, host security, and MCP
client behavior are outside this project's control.

Bug fixes target the default branch. Support for a tagged version must be stated
in that version's release notes.

[issues]: https://github.com/lennrt/tempestkeep/issues
