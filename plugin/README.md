# TempestKeep plugin

This package connects `tempestkeep mcp` to Claude Code. It contains three
commands and one station-analysis skill. Other MCP clients can use the server
directly without this plugin.

## Prepare the server

Build `tempestkeep` from the repository root with Go 1.27.0:

```sh
make tempestkeep
```

The plugin's [MCP configuration](.mcp.json) runs `tempestkeep mcp`. It finds the
executable through the client's `PATH`. It does not install or build the
executable and does not include credentials.

Configure `TEMPEST_TOKEN` and an absolute `TEMPEST_DB` path in the client's
private environment. For an existing archive without live access, omit the
token and set `TEMPEST_READ_ONLY=true`. See the [MCP guide](../docs/mcp.md)
for the full configuration and capability rules.

Do not paste a token into chat or place it in a command argument. A private
`.env` is also supported, but the server reads it from its working directory.
A desktop client's working directory can differ from your terminal's directory.

## Try the local plugin

After preparing the environment, run from the repository root on a Unix-like
system:

```sh
export PATH="$(pwd)/bin:$PATH"
claude --plugin-dir ./plugin
```

This loads the local plugin for that Claude Code session. The command follows
Claude Code's [local plugin workflow](https://code.claude.com/docs/en/plugins).
It requires a Claude Code installation that supports `--plugin-dir`.

In the session, inspect the `tempestkeep` MCP connection and tool list.
For an existing archive, call `archive_status`. For live access, call
`current_conditions` and inspect its source and timestamp.

The repository also contains [marketplace metadata](../.claude-plugin/marketplace.json).
Use Claude Code's [installation guide](https://code.claude.com/docs/en/discover-plugins)
for persistent installation. Review the source revision and client permissions
before installation or distribution.

## Included commands and skill

| Item | Result |
|---|---|
| `/tempestkeep:setup` | Guides server connection, private configuration, and first data. |
| `/tempestkeep:report` | Produces a report using the live and archive tools that are available. |
| `/tempestkeep:build-archive` | Runs a bounded number of backfill and sync calls, then reports remaining work. |
| `station-analyst` | Selects archive tools and accounts for units, dates, and missing readings. |

The commands guide the agent. They do not add a background collector or a
scheduler. To update the archive later, run the command again or call
`tempestkeep collect` from a terminal.

For troubleshooting, see [server and configuration problems](../docs/troubleshooting.md).
For worked analysis examples, see the [query guide](../docs/querying.md).
TempestKeep is independent of WeatherFlow and the Tempest weather platform.
