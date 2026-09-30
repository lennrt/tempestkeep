# Troubleshooting

Start with `tempestkeep version` and `tempestkeep help <command>`. For MCP,
inspect the client's server status and stderr log. Logs and tool results can
contain different information, so keep both private until you remove sensitive
details.

## The MCP server does not connect

Use an absolute executable path in the client configuration. A desktop client
does not always inherit your terminal's `PATH`. Make sure that the configured
arguments include `mcp` and that the client can run the executable.

If the server reports `no data source`, configure a token or an existing archive.
For a new archive, provide both a token and an explicit `TEMPEST_DB` path.
Without a path, MCP uses `./tempest.sqlite` only when that file already exists.

If the configured archive cannot open, make sure that the path identifies the
intended local file. A relative path starts at the client's working directory.
An explicit invalid archive blocks startup even when a token is available.

When you run `tempestkeep mcp` in a terminal, it waits for protocol input.
It does not display a dashboard. Use `tempestkeep now` for the dashboard.
Do not redirect stderr into stdout in an MCP launch command.

## Configuration changes do not take effect

Restart the process after changing its environment or `.env`. Commands read
`.env` only from the current working directory. `setup --env <path>` selects
the file to write, not a global file that later commands automatically find.

A process environment value takes precedence over `.env`, even when that
value is empty. If a terminal command works but MCP does not, compare the
client's environment and working directory. Do not print tokens while comparing
the configuration.

On Unix-like systems, a `.env` file must grant no access to the group or others:

```sh
chmod 600 .env
```

The file must be a regular file, not a symlink. Duplicate keys, invalid quotes,
and malformed assignments are errors. Use one `KEY=VALUE` assignment per line
and put comments on separate lines. The parser does not expand shell variables.

## The archive write tools are absent

Write tools require a token, an archive path, and write access enabled.
Either `--read-only` or a true `TEMPEST_READ_ONLY` removes `backfill_archive`
and `sync_archive`. A false flag cannot override a true environment value.

Read-only mode keeps `archive_status` and all available archive queries.
It also keeps live tools when a token exists. To run without live access,
remove the token from both the process environment and `.env`.

If `sync_archive` reports no stored data, call `backfill_archive` first.
Sync extends an existing archive after its newest observation.

## Collection stops or reports a device mismatch

After an interruption, rerun the same collection command. Committed chunks
remain in the archive. An open-ended backward collection resumes from its
saved cursor, and duplicate observations do not create extra rows.

An archive belongs to one device. If you intend to collect another device,
choose a new archive path. Do not remove the stored device binding to force
mixed observations into one file.

CLI collection supports `--device-id` and `TEMPEST_DEVICE_ID`. MCP selects
the first Tempest device found for its token. With access to several stations,
make sure that your collection method selects the device that owns the archive.

Repeated `sync_archive` calls retrieve new data, not historical gaps. For a
gap, use the bounded `backfill_archive` example in the [MCP guide](mcp.md#build-an-archive).
WeatherFlow can return no data for a gap that the API cannot recover.

If collection reports invalid checkpoint metadata, preserve the archive and
its metadata. This includes the backward cursor, completion flag, empty-history
state, and forward sync cursor. Do not delete or edit those values to bypass
the error. They determine which history a later run requests. Use a known-good
backup or report the sanitized error through [SUPPORT.md](../SUPPORT.md).

## A backup fails after collection

A backup error does not undo committed observations. Make sure that the local
filesystem has space and that the `backups` directory beside the archive is
writable by the current user. Do not delete unrelated files to satisfy backup
rotation.

The backup operation uses a filesystem hard link to publish a completed copy
without replacing an existing snapshot. The filesystem must support that
operation. `--backup-keep 0` disables post-collection backups for a run if you
use another backup method.

Keep the active database on a local filesystem. Stop writers before copying
the database manually. Transfer a completed backup or export between machines.

## Recover an archive from a backup

Use a completed SQLite backup for recovery. CSV and JSON Lines exports omit
collector metadata and some stored columns. TempestKeep does not provide an
export-import command that reconstructs the archive.

1. Stop TempestKeep processes that write to the affected archive.
2. Copy a known-good backup to a new, unused local archive path.
3. Run `tempestkeep stats --db <restored-path>` to inspect its coverage and records.
4. Point `TEMPEST_DB` at the restored path and restart its MCP client or collector.
5. Run collection and inspect coverage before relying on the restored archive.

Keep the original archive until you finish inspecting the restored copy.
Collection can request data newer than the backup, but WeatherFlow availability
limits what it can recover. An explicit historical gap still needs a targeted
backfill.

## Dates, totals, or recent conditions look wrong

Inspect `current_conditions.source`, `time`, and `age_seconds`. If live access
fails, an archive can supply an older observation. Cached live data can also
be up to the configured cache lifetime old.

Calendar reports use the process timezone. On Unix-like systems, set `TZ`
before starting the process. Native Windows uses the host timezone.
See the [query guide](querying.md#dates-and-timezones) before comparing daily
or monthly results across hosts.

Raw SQL and default exports use SI units. History tools and `stats` use US
display units for temperature, wind, pressure, and rain. Archived pressure is
station pressure, while live conditions can use sea-level pressure.

Inspect sample counts and sensor availability before interpreting a zero,
an extreme, or a trend. Missing rows are not fair-weather observations.
The [query guide](querying.md) explains aggregation and coverage limits.

## The terminal view shows a resize notice

The current-conditions view needs 61 columns. The archive explorer needs
68 columns. Enlarge the terminal or use `now --once`, `now --format json`,
`stats`, or `export` for output that does not need an interactive dashboard.

For short terminals, use Up/Down or Page Up/Page Down to scroll. Resizing
returns to the top. Press `q`, Esc, or Ctrl+C to exit the dashboard.

If these steps do not resolve the issue, follow [SUPPORT.md](../SUPPORT.md).
Use synthetic examples and remove credentials, private paths, and station
identifiers before posting diagnostics.
