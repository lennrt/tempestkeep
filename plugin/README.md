# TempestKeep plugin

This package connects the `tempest-mcp` command to Claude Code. It also contains
three commands and one station-analysis skill. The MCP server remains a standard
stdio server and does not depend on the plugin.

## Prerequisites

Build `tempest-mcp` from the repository with Go 1.27.0:

```sh
make mcp
```

Place `bin/tempest-mcp` on `PATH`. Configure `TEMPEST_TOKEN` and `TEMPEST_DB` in
the client environment. Do not paste a token into chat or place it in a command
argument.

## Included commands and skill

| Item | Result |
|---|---|
| `/tempestkeep:setup` | Checks server, token, archive, and first data in order. |
| `/tempestkeep:report` | Produces a short live and historical report. |
| `/tempestkeep:build-archive` | Runs a bounded set of archive backfill calls. |
| `station-analyst` | Selects archive tools for history questions. |

Review the manifest, marketplace metadata, permission behavior, and install flow
before installation or distribution.

TempestKeep is independent and is not affiliated with or endorsed by WeatherFlow
or the Tempest weather platform.
