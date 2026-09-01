# ADR 0002: Use one TempestKeep command and a project tap

## Status

Accepted before the first stable release.

## Context

The initial repository built `tempest` for terminal use and `tempest-mcp` for
MCP clients. The `tempest` name is already used by the
[OpenStack Tempest command](https://docs.openstack.org/tempest/latest/overview.html).
Installing a generic name into a shared executable directory can shadow an
unrelated program.

Homebrew core applies project-age and notability gates. TempestKeep does not need
core distribution to provide a normal Homebrew installation. A project-owned tap
can publish the formula under the unique `tempestkeep` name.

## Decision

Build and release one executable named `tempestkeep`. Run normal operations as
`tempestkeep <command>`. Run the MCP server as `tempestkeep mcp`.

Do not install compatibility aliases or symlinks named `tempest` or
`tempest-mcp`. The Homebrew formula name and installed executable are both
`tempestkeep`. The distribution target is the project-maintained
`lennrt/homebrew-tap`, not `Homebrew/homebrew-core`.

Keep these existing names because they describe the compatible product,
configuration, storage, or protocol rather than a global executable:

- `TEMPEST_*` environment variables;
- `tempest.sqlite` archive filenames;
- `tempest://` MCP resource URIs; and
- `github.com/lennrt/tempestkeep/pkg/tempest` Go package paths.

The command layer parses MCP flags and configuration. It passes an
interrupt-aware context and explicit options to `internal/mcpapp`. The MCP
package does not read command arguments or retain a context.

## Compatibility

This is an intentional breaking operational change:

- `tempest <command>` becomes `tempestkeep <command>`;
- `tempest-mcp [flags]` becomes `tempestkeep mcp [flags]`;
- MCP client launch configuration changes; and
- release archives contain one executable instead of two.

The MCP initialization server name changes from `tempest-mcp` to `tempestkeep`.
MCP tool names, JSON fields, resource URIs, environment variables, the SQLite
schema, and exported Go APIs do not change.

The change occurs before the first stable release. No compatibility executable
is retained because that would preserve the name collision this decision fixes.

## Consequences

Users install one unique command. Homebrew needs one formula output and no
`conflicts_with`, `link_overwrite`, service, or compatibility symlink. MCP clients
set the command to `tempestkeep` and the first argument to `mcp`.

Release qualification must confirm that every archive contains exactly one
platform executable plus the required notices and documentation. The tap must
build from an immutable source release and must test without a live token.
