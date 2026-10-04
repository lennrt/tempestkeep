---
description: "Guided setup for the token, archive, MCP connection, and first data"
argument-hint: ""
---

Run each phase in order. Reuse configuration and authorization already supplied
by the user. Ask only for missing choices. Do not ask the user to paste a token
into chat or expose it in tool arguments.

## Establish the connection

Inspect the MCP client's `tempestkeep` server status. If the executable is
missing, build it from the repository root:

```sh
make tempestkeep
```

Place `bin/tempestkeep` on the client's `PATH` or configure its absolute path.
The server arguments must include `mcp`. Restart the MCP client after changing
its launch configuration.

If startup reports `no data source`, configure live access or an existing
archive. If startup cannot open the configured archive, make sure that the
path and access permissions are correct. Use an absolute local path.

## Configure private access

If the user needs live access and has no token, give these steps:

1. Sign in at [tempestwx.com](https://tempestwx.com).
2. Open Settings > Data Authorizations > Create Token.
3. Save the token in the MCP client's private environment as `TEMPEST_TOKEN`.

Do not request the token's value. For terminal use, a private `.env` file in
the working directory is also supported. On Unix-like systems, it must grant
no access to the group or others. A desktop client can use a different working
directory, so prefer its private environment.

An existing archive supports offline reads without a token. Preserve that
choice when the user wants archive-only access. For read-only MCP operation,
set `TEMPEST_READ_ONLY=true`. This keeps live reads if a token still exists.

## Select the archive

Use the path already provided by the user, or ask where to store the archive.
Choose a stable local directory and an absolute `TEMPEST_DB` path. Do not place
an active SQLite database in a cloud-sync folder. Transfer a completed backup
or export between machines.

With a token and write access, the MCP server creates a new archive at the
configured path. Without write access, the archive must already exist.
Restart the client after changing the environment.

Explain that each archive belongs to one Tempest device. MCP selects the first
Tempest device found for the token. `TEMPEST_DEVICE_ID` changes CLI collection
only, so it does not choose the MCP device.

## Test the configured sources

For live access, call `list_stations` and require a successful response.
Then call `current_conditions` and report a short result with its source and
timestamp. Do not paste coordinates, serial numbers, or identifiers into the
conversation unless the user asks for them.

For archive access, call `archive_status`. If the archive contains rows,
call `current_conditions`. If it is empty, explain that the first backfill
supplies history. Empty history is not a successful current reading.

If live authorization fails, direct the user to update the private token.
If an archive cannot open, resolve the path or access error before proceeding.
Do not treat an archive fallback as proof that the token works.

## Collect initial history

If the user already requested collection, run the bounded
`/tempestkeep:build-archive` workflow. Otherwise, offer that workflow before
starting network collection. Stop when its call budget ends or a tool fails.

Report coverage, freshness, and any remaining work. Explain that later runs
resume saved progress. The plugin does not schedule background collection.
End with example questions that the available data can answer, such as a gust
record, a monthly comparison, or a daily temperature summary.
