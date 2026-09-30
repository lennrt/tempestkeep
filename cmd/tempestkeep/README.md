# `tempestkeep`

`tempestkeep` configures, collects, reads, exports, and displays Tempest data.
It also serves live and archived data over MCP stdio. Each archive belongs to
one device.

## Build

Use Go 1.27.0 from the repository root:

```sh
make tempestkeep
```

The result is `bin/tempestkeep`. The examples below run from the repository
root. If you place the executable on `PATH`, use `tempestkeep` without the
`bin/` prefix.

## Commands

```text
tempestkeep setup          Configure a token and archive.
tempestkeep list-devices   List station and device data visible to the token.
tempestkeep collect        Create or update one device archive.
tempestkeep now            Show live or archived current conditions.
tempestkeep explore        Browse archive periods and records.
tempestkeep stats          Print an archive report.
tempestkeep export         Write CSV or JSON Lines.
tempestkeep mcp            Serve live and archived data over MCP stdio.
tempestkeep version        Print the installed version.
tempestkeep help           Show help.
```

Run `tempestkeep help <command>` for flags and examples. Usage errors exit
with status 2. Runtime failures exit with status 1. Successful commands and
help exit with status 0.

## Setup

Run the wizard in an interactive terminal:

```sh
bin/tempestkeep setup
```

The wizard tests the token against WeatherFlow, writes private configuration,
and provides MCP setup guidance. By default, it writes `.env` in the current
directory. `--env <path>` selects another output file. Later commands still
look for `.env` in their own current directory.

On Windows, the wizard's generated shell commands target PowerShell. Keep
their single-quote escaping when paths contain spaces or apostrophes.

For manual setup on Unix-like systems, copy the example and restrict access
before entering a token:

```sh
cp .env.example .env
chmod 600 .env
```

Edit `.env` in a local editor. Set `TEMPEST_TOKEN` and an absolute `TEMPEST_DB`
path. Do not place a token in a command argument or paste it into chat.
On Windows, restrict access through the file's permissions for your account.

To inspect available devices, run:

```sh
bin/tempestkeep list-devices
```

This output includes station and device identifiers. Keep it private. If your
token can access several devices, select the intended Tempest device for
collection with `--device-id` or `TEMPEST_DEVICE_ID`.

## Collection

After setup, run:

```sh
bin/tempestkeep collect
```

On a new archive, the command walks backward through available history. An
interrupted backward walk resumes on the next run. Once backfill completes,
later runs sync after the newest stored observation, called the watermark.

To set an oldest date on a new archive, use:

```sh
bin/tempestkeep collect --backfill-start 2025-01-01
```

The date starts at midnight in the process timezone. The flag does not request
an older gap repair on an archive that already has a watermark. For that case,
use an explicit `backfill_archive` window through [MCP](../../docs/mcp.md#build-an-archive).

Each API request covers at most five days. Each successful chunk commits before
the next request. The `(device_id, epoch)` key prevents duplicate rows when a
chunk is replayed. Existing rows are not replaced by a later API response.

Progress goes to stderr. `--quiet` disables progress but leaves errors visible:

```sh
bin/tempestkeep collect --quiet
```

SIGINT and SIGTERM cancel collection. An interrupted run exits with a failure
status and preserves committed rows and resume metadata. Rerun the same command
to continue.

After successful collection, the command checkpoints SQLite's write-ahead log
(WAL), which holds recent database changes. It then closes the writer and makes
a backup. Backups live in `backups` beside the archive, not necessarily beside
the executable or working directory.

The default retention is seven snapshots. `--backup-keep` accepts 0 through
365, and zero disables backups. `--no-backup` skips the snapshot for that run.
A backup failure returns an error without undoing collected data. The backup
filename uses a UTC timestamp and never replaces an existing snapshot.

Keep the active archive on a local filesystem. Stop other writers before a
backup or manual copy. Move a completed backup or export between machines.
See [troubleshooting](../../docs/troubleshooting.md#a-backup-fails-after-collection)
for backup errors.

## Current conditions and exploration

`now` prefers live data when a token exists. If live data fails and an archive
exists, it can display the latest archived observation. Read the timestamp and
source note before treating that observation as current.
An archived fallback uses the discovered station name only when the archive's
device belongs to that station. Otherwise, the station name remains absent.

Use a single frame or JSON for scripts:

```sh
bin/tempestkeep now --once
bin/tempestkeep now --format json
```

`explore` reads only the archive:

```sh
bin/tempestkeep explore --db /absolute/path/to/tempest.sqlite
```

Use Left/Right to move between periods. Use `d/w/m/y/r` to select a view.
Use Tab to change the month or year metric. Use Up/Down and Page Up/Page Down to scroll.
Press `q`, Esc, or Ctrl+C to exit. See the [terminal controls](../../README.md#terminal-controls)
for refresh keys and minimum widths.

Color is disabled when stdout is not a terminal. `NO_COLOR`, `TERM=dumb`, and
`--no-color` also disable color. `stats` and `export` provide output that does
not require the interactive explorer.

## Archive reports

For a report covering a selected month, run:

```sh
bin/tempestkeep stats --start 2025-06-01 --end 2025-06-30
bin/tempestkeep stats --start 2025-06-01 --end 2025-06-30 --format json
```

Both dates are local calendar dates. The end date includes the entire day.
Without a start date, the range starts at the beginning of the archive.
Without an end date, the range ends at the current time.

The range applies to rain, wind, lightning, solar, pressure, comfort, and
weather-spell sections. Coverage, records, and temperature trend always
summarize the whole archive. This remains true in JSON output.

Reports use US display units for temperature, wind, pressure, and rain.
The command reads the archive without contacting WeatherFlow. See the
[query guide](../../docs/querying.md) before interpreting gaps or trends.

## Export observations

`export` writes stored observations as CSV or JSON Lines, with one object per
line. It reads only the archive and does not contact WeatherFlow. The default
format is CSV and the default units are SI.

For a local calendar month in US units, run:

```sh
bin/tempestkeep export --start 2025-06-01 --end 2025-06-30 --units us
```

For JSON Lines in SI units, run:

```sh
bin/tempestkeep export --format jsonl
```

If `jq` is installed, this example selects each observation's temperature in °C:

```sh
bin/tempestkeep export --format jsonl | jq .air_temp_c
```

The date bounds follow the same inclusive local-day rules as `stats`.
`epoch` contains UTC seconds, and `time` contains an RFC3339 timestamp with
the local UTC offset. Column names identify each measurement's units.

CSV represents missing sensor readings as empty fields. JSON Lines omits the
corresponding key. A missing temperature is not 0°C. See the
[query guide](../../docs/querying.md#units-and-aggregation) for interval totals
and aggregation rules.

Exports contain private weather observations. For a saved export on Unix-like
systems, use a protected destination and an owner-only creation mask:

```sh
umask 077
bin/tempestkeep export --format jsonl > /private/existing/directory/observations.jsonl
```

Replace the destination with an existing private directory. The shell creates
the output file, so TempestKeep does not set its permissions. If the command
fails, the file can contain partial output. Check the exit status before using
it as a complete export.

## Configuration

Commands load `.env` only from their current working directory. The process
environment takes precedence, including values explicitly set to empty.
An explicit nonempty path flag takes precedence over either source.
See [`.env.example`](../../.env.example) for supported variables and bounds.

For numeric collection flags, an explicit value overrides the environment.
For MCP read-only mode, either the flag or a true environment value disables
writes. `TEMPEST_READ_ONLY` does not disable CLI `collect`.

A `.env` file contains literal `KEY=VALUE` assignments. Single and double
quotes are supported. Double-quoted values use Go-style escapes, so use
single quotes around Windows paths with backslashes. Put comments on their
own lines. Shell variable expansion and inline comments are not supported.

On Unix-like systems, `.env` must grant no group or other access. The file
must be regular, not a symlink. Malformed assignments and duplicate keys fail
instead of silently choosing a value.

`collect` defaults to `./tempest.sqlite` and can create it. Other commands
select that default file only when it exists. For a new MCP archive, set an
explicit path and provide a token. Use absolute paths when launching commands
from different directories or from a desktop client.

On Unix-like systems, set `TZ` before starting calendar reports when the host
and station use different timezones. Native Windows uses the host timezone.
See [date and timezone rules](../../docs/querying.md#dates-and-timezones).

See the [root quickstart](../../README.md), [MCP guide](../../docs/mcp.md), and
[troubleshooting guide](../../docs/troubleshooting.md). TempestKeep is
independent of WeatherFlow and the Tempest weather platform.
