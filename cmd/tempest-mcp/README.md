# `tempest-mcp`

`tempest-mcp` serves live and archived Tempest data over MCP stdio.

## Build

Use Go 1.27.0 from the repository root:

```sh
make mcp
```

The result is `bin/tempest-mcp`.

## Configure

Provide configuration through the MCP client's environment:

| Variable | Default | Bound and result |
|---|---|---|
| `TEMPEST_TOKEN` | empty | Enables live API operations and archive collection. Treat it as a secret. |
| `TEMPEST_DB` | `./tempest.sqlite` when present | Selects one local SQLite archive. |
| `TEMPEST_READ_ONLY` | false | `1`, `true`, `yes`, or `on` removes archive write operations. |
| `TEMPEST_CACHE_TTL` | 300 | Live cache seconds from 0 through 86400. |
| `TEMPEST_THROTTLE_MS` | 400 | Collection delay in milliseconds from 0 through 60000. |
| `TEMPEST_API_BASE` | WeatherFlow endpoint | Test or proxy endpoint. Use HTTP or HTTPS without credentials, query, or fragment. |

Do not pass a token as `--token`. A command argument can remain in shell history
or process diagnostics. Use a private `.env` file or the MCP environment.

## Capabilities

| Available input | Registered capability |
|---|---|
| token | live conditions, station list and details, and forecast |
| archive | archived conditions, summaries, records, sensor analysis, and read-only SQL |
| token and writable archive | bounded backfill, sync, and archive status |

`--read-only` removes write operations even when a token and archive exist.

Archive queries use `store.Store` with SQLite read-only mode and
`PRAGMA query_only`. The SQL operation accepts one `SELECT` or `WITH ... SELECT`
and limits query bytes, execution time, columns, rows, and result bytes.

Archive writes use `store.Writer`. Observation inserts are append-only and use
`INSERT OR IGNORE` on `(device_id, epoch)`. Resume metadata can be updated. The
server does not expose arbitrary write SQL.

## Run

Start an archive-only server:

```sh
bin/tempest-mcp --db ./tempest.sqlite
```

The process waits for MCP JSON-RPC on stdin. Stdout carries JSON-RPC only.
Diagnostics use stderr. Stop it by canceling the MCP session or sending an
interrupt.

If no token and no readable archive exist, startup fails. A live API outage does
not block startup. The first live call resolves the station and retries resolution
after a failure.

## Limits

- Forecast output is capped by the tool schema and API response limits.
- Observation series and SQL results have row, point, column, time, and byte
  limits in `pkg/tempest/store`.
- One archive belongs to one device.
- Calendar results use the process timezone.
- Live operations depend on WeatherFlow availability and token permissions.

See [the architecture](../../docs/architecture.md),
[the threat model](../../docs/threat-model.md), and
[the property catalog](../../docs/testing/properties.md).

TempestKeep is independent and is not affiliated with or endorsed by WeatherFlow
or the Tempest weather platform.
