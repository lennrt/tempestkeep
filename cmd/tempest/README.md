# `tempest`

`tempest` configures, collects, reads, exports, and displays Tempest data.

## Build

Use Go 1.27.0 from the repository root:

```sh
make tempest
```

The result is `bin/tempest`.

## Commands

```text
tempest setup          Configure a token and archive.
tempest list-devices   List station and device data visible to the token.
tempest collect        Create or update one device archive.
tempest now            Show live or archived current conditions.
tempest explore        Browse archive periods and records.
tempest stats          Print an archive report.
tempest export         Write CSV or JSON Lines.
tempest version        Print the installed version.
tempest help           Show help.
```

Run `tempest help <command>` for prerequisites, flags, limits, and examples.
Usage errors exit with status 2. Runtime failures exit with status 1. Help exits
with status 0.

## Setup

Run:

```sh
bin/tempest setup
```

The wizard validates the token and writes selected configuration. If you create
`.env` manually, set owner-only permissions:

```sh
cp .env.example .env
chmod 600 .env
```

Do not place a token in a command argument.

## Collection

Run `tempest collect` on a new archive to walk backward through available
history. Run it again to sync from the stored watermark. Use
`--backfill-start YYYY-MM-DD` to set an explicit oldest date on a new archive.

Each request covers at most five days. Each successful chunk commits before the
next request. Replayed observations do not create duplicates. An interrupted
open-ended collection stores its cursor and resumes on the next run.

Progress uses stderr. `--quiet` disables progress. Errors still use stderr.

After a successful collection, the command checkpoints the WAL and creates a
private backup. `--backup-keep` accepts 0 through 365. A value of 0 disables
backups. Backup failure returns an error and leaves the archive intact.

## Display and export

`tempest now` uses live data when a token exists. It can fall back to the latest
archive row. Use `--once` for one frame and `--format json` for structured output.

`tempest explore` reads the archive only. Use arrow keys to move between periods,
`d`, `w`, `m`, `y`, or `r` to select a view, `tab` to change the heatmap metric,
and `q` to exit.

`tempest stats` writes text or JSON to stdout. `tempest export` streams CSV or
JSON Lines to stdout. Redirect those outputs only to a protected location; they
contain station observations.

Color is disabled when stdout is not a terminal. `NO_COLOR`, `TERM=dumb`, and
`--no-color` also disable color.

## Configuration precedence

An explicit flag overrides the environment. The environment overrides `.env`.
See `.env.example` for variables and defaults.

The active archive must use a local filesystem. Stop writers before moving it.
Move a completed backup or export, not an active WAL database.

See [the root quickstart](../../README.md) and
[the architecture](../../docs/architecture.md).

TempestKeep is independent and is not affiliated with or endorsed by WeatherFlow
or the Tempest weather platform.
