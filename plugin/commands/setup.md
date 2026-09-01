---
description: "Guided first-run setup: token, archive location, MCP wiring, first data"
argument-hint: ""
---

Run each phase in order. Ask one question at a time. Verify each result with a
tool call. Skip a phase only after you verify its result.

<phases>

## 1. Check the server

Confirm that the MCP client connected to the `tempestkeep` server.

- If the client reports that `tempestkeep` is not executable, build and install
  the binary. In a local checkout, run:

  ```
  make tempestkeep
  ```

  Put `bin/tempestkeep` on `PATH`. Restart the MCP client.
- If startup reports "no data source," go to phase 2 or configure an existing
  archive in phase 3.
- If startup cannot open the configured archive, correct or remove `TEMPEST_DB`.
- If the server connects, call `list_stations` when that tool is available. A
  successful call proves that the configured token works. An authorization
  failure returns to phase 2. If the tool is absent, the server is archive-only;
  ask whether the user wants live access before continuing.

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

For live access, call `list_stations` and require a successful result. Then call
`current_conditions` and show one line with the result. In archive-only mode,
call `archive_status` before `current_conditions`. A missing configured archive
is a startup error; correct or remove `TEMPEST_DB` before continuing.

## 5. Offer to build the history

Ask if the user wants to build the local archive. If yes:

1. Call `backfill_archive` with no arguments. Each call processes one bounded,
   resumable batch.
2. Call it at most 12 times. Stop earlier on completion or failure. Report the
   resume state if work remains.
3. Call `archive_status`. Report the coverage span, row count, and one result from
   `records`.

If the user declines, state that `tempestkeep collect` builds the same archive
from a terminal.

</phases>

End with three example questions, such as "What is my record gust?", "Compare this
July to last July," and "When does my yard get the most sun?"
