---
description: "Guided first-run setup: token, archive location, MCP wiring, first data"
argument-hint: ""
---

Run each phase in order. Ask one question at a time. Verify each result with a
tool call. Skip a phase only after you verify its result.

<phases>

## 1. Check the server

Call the `tempest` server's `current_conditions` tool (or `archive_status`).

- If a tool returns data, go to phase 4.
- If a tool reports "no token," go to phase 2.
- If no `tempest` tools exist, build and install the binary. In a local checkout,
  run:

  ```
  make mcp
  ```

  Put `bin/tempest-mcp` on `PATH`. Restart the MCP client.

## 2. Personal access token

Ask if the user has a WeatherFlow personal access token. If not, give these steps:

1. Sign in at [tempestwx.com](https://tempestwx.com).
2. Open **Settings > Data Authorizations > Create Token**.
3. Name and copy the token.

Do not ask the user to paste the token into chat. Set `TEMPEST_TOKEN` in the MCP
client's private environment. For local terminal use, a private `.env` file is
also supported:

```
TEMPEST_TOKEN=<their token>
```

## 3. Where should the archive live?

Explain the choices in one short paragraph. Then ask where to store the SQLite
archive.

- **User data directory**: stable across working directories and
  private to this machine.
- **Current directory**: `./tempest.sqlite`, useful when the archive belongs to a
  specific project.
- **Custom local path**: any existing local directory. Do not put the active
  SQLite database in a cloud-sync folder; copy a closed backup or export instead.

Set `TEMPEST_DB=<absolute path>` in the same private environment or `.env` file.
Restart the MCP client to load the setting.

## 4. Prove it works

Call `current_conditions`. Show one line with the result. If the token is invalid,
return to phase 2. A missing configured archive is a startup error; correct or
remove `TEMPEST_DB` before continuing.

## 5. Offer to build the history

Ask if the user wants to build the local archive. If yes:

1. Call `backfill_archive` with no arguments. Each call processes one bounded,
   resumable batch.
2. Call it at most 12 times. Stop earlier on completion or failure. Report the
   resume state if work remains.
3. Call `archive_status`. Report the coverage span, row count, and one result from
   `records`.

If the user declines, state that `tempest collect` builds the same archive from a
terminal.

</phases>

End with three example questions, such as "What is my record gust?", "Compare this
July to last July," and "When does my yard get the most sun?"
