# `tempestkeep mcp`

`tempestkeep mcp` serves live and archived Tempest data over MCP stdio.
The client starts the process and sends JSON-RPC messages through its standard
input and output. This command does not start an HTTP server or open a port.

## Build and connect

Use Go 1.27.0 from the repository root:

```sh
make tempestkeep
```

The result is `bin/tempestkeep`. Use its absolute path in the MCP client.
See the [client configuration example](../README.md#configure-the-mcp-client)
for a complete `mcpServers` entry.

Provide configuration in the client's private environment:

| Variable | Default | Meaning |
|---|---|---|
| `TEMPEST_TOKEN` | Empty | Enables live API operations. With a writable archive, it also enables collection. |
| `TEMPEST_DB` | `./tempest.sqlite` if that file exists | Selects one local SQLite archive. Set an explicit path to create a new archive with live access. |
| `TEMPEST_READ_ONLY` | False | `1`, `true`, `yes`, or `on` removes archive write tools. Values are case-insensitive. |
| `TEMPEST_CACHE_TTL` | 300 | Seconds to cache live API responses, from 0 through 86400. Zero disables caching. |
| `TEMPEST_THROTTLE_MS` | 400 | Delay between collection requests, from 0 through 60000 milliseconds. Zero disables the delay. |
| `TEMPEST_API_BASE` | WeatherFlow endpoint | Alternate API endpoint for tests or a trusted proxy. |
| `TZ` | Host timezone | Process timezone on Unix-like systems. Use the station's IANA name, such as `America/Los_Angeles`. |

The server also reads `.env` from its working directory. The client often uses
a different directory from your terminal. Prefer explicit environment values
and absolute archive paths. See the
[configuration rules](../cmd/tempestkeep/README.md#configuration) for file
syntax, permissions, and precedence.

Do not pass a token as a command argument. Use the client's secret store or
private environment. An alternate API endpoint receives the token. Use only
synthetic tokens with plaintext HTTP test endpoints.

`--db` selects an archive. `--read-only` disables archive writes. Either
`--read-only` or a true `TEMPEST_READ_ONLY` disables writes, so
`--read-only=false` cannot override a true environment value. Read-only mode
still permits live API calls when a token exists.

## Capabilities

The client discovers the capabilities that match the current configuration:

| Available input | Registered operations |
|---|---|
| Token only | `current_conditions`, `list_stations`, `station_details`, and `forecast` |
| Archive only | Archived conditions, `archive_status`, `station_info`, history tools, and `query_sql` |
| Token and archive | All live and archive reads |
| Token and writable archive | All reads, plus `backfill_archive` and `sync_archive` |

An archive enables two resources and three workflow prompts. The resources are
`tempest://archive/schema` and `tempest://archive/data-dictionary`. The prompts
are `weather_report`, `climate_review`, and `build_archive`. A prompt does not
enable tools that the configuration excludes.

With all capabilities enabled, the server exposes 29 tools. Use the client's
tool discovery to determine availability. Do not assume that every deployment
has live access or write access.

The server selects the first Tempest device found for the token. MCP does not
use `TEMPEST_DEVICE_ID`, which applies to CLI collection. If a token can access
multiple devices, make sure that the selected device matches the archive.
The archive rejects writes for a different device.

`station_info` adds live station metadata only when the archive's device
belongs to that station. Missing or mismatched identity leaves those fields
absent. Do not assume that live weather and a configured archive describe the
same station. See the [query guide](querying.md) before combining their results.

## Start and stop

For an existing archive, use this command without `TEMPEST_TOKEN` in the process
environment or `.env`:

```sh
bin/tempestkeep mcp --db /absolute/path/to/tempest.sqlite --read-only
```

For a new archive, configure a token and an explicit database path in the client.
Without `--read-only`, startup creates the archive and registers the write tools.
An empty archive supports discovery before the first collection.

The process waits for MCP input. A terminal with no output is expected.
Diagnostics go to stderr. Do not combine stderr with stdout in the client
launch command, because stdout must contain only JSON-RPC.

If no token and no archive exist, startup fails. If an explicit archive cannot
be opened, startup fails even when live access exists. A WeatherFlow outage does
not block startup. The first live call resolves the station, and later calls
can retry a failed resolution.

Cancel the MCP session or send SIGINT or SIGTERM to stop the server. Either
signal cancels active work and closes the archive. Committed observations
survive cancellation.

## Build an archive

The following JSON objects are tool arguments. Select the named tool in your
MCP client, then pass the object. They are not complete JSON-RPC messages.

Start with `archive_status`:

```json
{}
```

If the archive needs older history, call `backfill_archive`:

```json
{"max_days": 30}
```

After each successful call, inspect `rows_added`, `coverage`, `has_more`, and
`next_before`. If `has_more` is true, another call without dates resumes the
saved cursor. Stop on an error or when the client's time budget ends. A later
session can continue from the committed progress.

To stop at a chosen oldest date, pass `start` and leave `end` unset:

```json
{"start": "2025-01-01", "max_days": 30}
```

Repeat these same arguments while `has_more` is true. The `start` boundary
applies to each call. Omitting it on a later call resumes an open-ended walk
that can continue before the requested date.

Then call `sync_archive` with `{}` until `has_more` is false. Sync appends rows
after the newest stored observation. On an empty archive, sync asks you to
backfill first. It does not seed history or repair older gaps.

During a bounded catch-up, sync preserves progress through empty API windows
across calls and restarts. After catch-up completes, a later sync rechecks the
tail after the newest stored row for observations that arrived late.

Finish with `archive_status`. Its `gaps` field lists at most ten gaps longer
than one hour. The list does not measure every missing minute or missing sensor
value. `backfill_complete` records the end of a backward history walk. The
open-ended walk stops after 15 consecutive days of empty API results. A long
historical outage can therefore stop it before older data. An explicit `start`
lets the walk continue through empty ranges to that date.

Completion does not prove that every date has complete observations. A data
gap can remain even after the API's available history is exhausted.

For a specific gap, use an explicit window shorter than the call budget:

```json
{"start": "2025-06-01", "end": "2025-06-04", "max_days": 5}
```

This example includes June 1 through June 3 in the process timezone.
For `backfill_archive`, `end` is exclusive local midnight. An explicit `end`
does not update the shared resume cursor. Repeating the same window rereads
that window and ignores duplicate rows. Split larger repairs into bounded
windows instead of repeating an unchanged `end`.

The CLI and MCP use the same archive and collection metadata. MCP collection
does not create the CLI's post-collection backups. See the
[archive and backup guide](../cmd/tempestkeep/README.md#collection).

## Query history

For daily weather from June 1 through June 7, call `daily_summary`:

```json
{"start": "2025-06-01", "end": "2025-06-07"}
```

Unlike backfill, history query `end` dates include the entire end day.
For hourly detail on a day from that result, call `get_observations`:

```json
{"start": "2025-06-03", "end": "2025-06-03", "bucket_minutes": 60}
```

The result reports the applied `bucket_minutes`. It includes means for wind
and pressure, maxima for gusts and UV, and sums for rain and lightning.
Empty time buckets are absent. See the [query guide](querying.md) for units,
missing readings, comparison rules, and SQL examples.

## Limits and failure behavior

| Operation | Limit or default |
|---|---|
| `forecast` | Defaults to 24 hours and 10 days. Maximums are 240 hours and 10 days. |
| `daily_summary` | `days` defaults to 7 and accepts at most 366. Explicit `start` replaces `days`. |
| `get_observations` | `max_points` defaults to 288 and accepts at most 2000. The server can enlarge an explicit bucket to respect 2000 points. |
| `query_sql` | One `SELECT` or `WITH ... SELECT`. At most 64 KiB of SQL, 128 columns, and 15 seconds of query execution. |
| SQL result | `max_rows` defaults to 100 and accepts at most 1000. Result data is limited to 8 MiB before JSON encoding. |
| `backfill_archive` | `max_days` defaults to 30 and accepts at most 365. Each API request covers at most five days. |
| `sync_archive` | Each call processes at most six five-day requests. Repeat while `has_more` is true. |

Input and result limits also apply in the shared store. A caller deadline can
end an operation before its own limit. A bounded collection call is a work
limit, not a promise that the call will finish within a client's timeout.

`max_days` counts 24-hour periods. A batch can fetch fewer days to fit whole
request windows. For example, a six-day budget permits one five-day request.
A one-day budget permits a one-day request. Repeat without `end` while
`has_more` is true to continue.

Archive SQL uses a read-only SQLite connection with `PRAGMA query_only`.
It cannot change the archive. If a result reports `truncated: true`, narrow
the query or aggregate it before drawing conclusions.

See [troubleshooting](troubleshooting.md), [architecture](architecture.md),
and the [threat model](threat-model.md). TempestKeep is independent of
WeatherFlow and the Tempest weather platform.
