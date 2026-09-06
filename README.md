# TempestKeep

[![Development version][version-badge]](CHANGELOG.md)
[![CI][ci-b]][ci]
[![OpenSSF Best Practices][best-practices-badge]][best-practices]
[![License][license-badge]](LICENSE)
[![Go version][go-badge]][go-install]
[![Go Reference][docs-badge]][go-docs]
[![GitHub stars][stars-badge]][stars]

**Your weather station, available to your agent.**

TempestKeep is a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/)
server for WeatherFlow Tempest. Give an agent live conditions, forecasts, and a
queryable local history. It can build the archive itself, resume interrupted
collection, and answer historical questions from SQLite.

One Go binary. MCP over stdio. A local archive you can also inspect with the CLI.

[![MCP discovery, archive backfill, chained queries, and offline reuse][mcp-demo]][mcp-video]

[Watch the MP4 clip][mcp-video] · [Run the demo locally](#try-it-without-a-station)
· [Connect your agent](#connect-your-agent) · [CLI and terminal UI](#cli-and-terminal-ui)

The clip uses synthetic weather and a scripted Go client. Discovery, tool calls,
archive writes, and the final session without an API token all run through the
real MCP server. No LLM or WeatherFlow account is needed to replay it.

## What your agent can do

With live access and a writable archive, TempestKeep exposes **29 typed tools,
2 resources, and 3 prompts**. Each tool has an input schema and structured output.

| Ask your agent | MCP workflow |
|---|---|
| "Build my station's archive and keep it current." | `archive_status` → resumable `backfill_archive` → `sync_archive` |
| "Which day had the strongest gust in the last 30 days?" | `daily_summary` → `get_observations` for that day |
| "Where does the wind usually come from?" | `wind_rose` over the local archive |
| "Is today's weather unusual for this station?" | `current_conditions` → `this_day_in_history` → `records` |
| "Explain the archive before writing a query." | Read the schema and data-dictionary resources, then use `query_sql` |

The `weather_report`, `climate_review`, and `build_archive` prompts package
common workflows. The resources explain the schema, units, and aggregation
rules so a client can work with the stored data.

Capabilities follow configuration:

| Inputs | Available operations |
|---|---|
| token | live conditions, station metadata, and forecast |
| archive | local observations, summaries, records, and read-only SQL |
| token and writable archive | bounded archive backfill and sync |

Use `--read-only` or `TEMPEST_READ_ONLY=true` to remove archive write tools.
SQL reads are bounded and enforced by a read-only database handle. Backfill
calls have bounded work and persist a resume cursor between calls.

See the [MCP guide](docs/mcp.md) for configuration, capabilities, and limits.

## Connect your agent

### Build

You need **Go 1.27.0**, a local filesystem for SQLite, and a WeatherFlow personal
access token for live data or collection. An existing archive works without a
token. The normal build is pure Go; race tests need a C toolchain.

```sh
git clone https://github.com/lennrt/tempestkeep.git
cd tempestkeep
go mod download
make build
```

The executable is `bin/tempestkeep`. Use its absolute path in a desktop MCP
client, or put it on that client's `PATH`.

### Configure the MCP client

For clients that use an `mcpServers` configuration, add the following to the
client's **private local configuration**. Replace the paths, token placeholder,
and timezone with your own values:

```json
{
  "mcpServers": {
    "tempestkeep": {
      "command": "/absolute/path/to/tempestkeep/bin/tempestkeep",
      "args": ["mcp"],
      "env": {
        "TEMPEST_TOKEN": "YOUR_WEATHERFLOW_TOKEN",
        "TEMPEST_DB": "/absolute/path/to/weather/tempest.sqlite",
        "TZ": "America/Los_Angeles"
      }
    }
  }
}
```

Use your client's secret store if it provides one. Keep credentials out of Git
and command arguments. Clients with a different configuration format need the
same command, arguments, and environment values.

Restart or reconnect the MCP client, then ask it to **build your station's local
archive**. With a token and database path, TempestKeep creates the archive and
exposes the backfill tools. For an existing archive without live access, omit
`TEMPEST_TOKEN` and add `--read-only` to `args`.

The host launches `tempestkeep mcp` and exchanges JSON-RPC over stdin/stdout.
Diagnostics go to stderr. SIGINT and SIGTERM cancel work and close the archive.
The archive remains local; tool results are delivered to the connected MCP
client and may be sent to the model that client uses.

The optional [Claude Code plugin](plugin/README.md) uses the same server.

## Try it without a station

Run the same MCP session shown above with a local synthetic API and a temporary
archive. No real token or model provider is used:

```sh
make mcp-demo
```

The demo discovers capabilities, builds 45 days of history, chains a daily
summary into an hourly query, and reconnects with no token in read-only mode.
It removes its temporary archive and stops the synthetic API when finished.

To record the GIF and MP4 with [Charm's VHS][vhs], install VHS v0.11.0, `ttyd`,
and `ffmpeg`, then run:

```sh
make demo-agent
```

The recording is defined in [docs/agent.tape](docs/agent.tape), with a
[reproduction guide](docs/demo.md) covering the transcript and recording checks.

## CLI and terminal UI

The CLI gives you direct access to the same live data and archive. Start with
the interactive setup wizard, then collect history and open the dashboard:

```sh
./bin/tempestkeep setup
./bin/tempestkeep collect
./bin/tempestkeep now
./bin/tempestkeep explore
```

For manual setup, copy `.env.example` to `.env`, set `TEMPEST_TOKEN`, and restrict
the file to the current user with `chmod 600 .env`.

```text
tempestkeep setup          Configure the token and archive.
tempestkeep list-devices   List stations and device identifiers.
tempestkeep collect        Create or update the archive.
tempestkeep now            Show current conditions and forecast.
tempestkeep explore        Explore archived days, months, and records.
tempestkeep stats          Print archive statistics.
tempestkeep export         Write CSV or JSON Lines to stdout.
tempestkeep mcp            Serve live and archived data over MCP stdio.
tempestkeep version        Print the installed version.
tempestkeep help           Show command help.
```

Use `tempestkeep help <command>` for flags, bounds, results, and failure behavior.
Machine-readable commands keep data on stdout and diagnostics on stderr.

![The current TempestKeep terminal dashboard](docs/tempest-now.svg)

## Terminal controls

The current-conditions card needs 61 columns; the explorer needs 68. Narrower
windows show a resize notice rather than a broken border. On short terminals,
use **Up/Down** (or **k/j**) and **Page Up/Page Down** to scroll. A position hint
appears when content extends beyond the screen. Resizing returns to the top.

In `now`, **r** refreshes or retries. In `explore`, **Enter** refreshes or retries,
**Left/Right** (or **h/l**) scrubs periods, **d/w/m/y/r** selects a view, **Tab**
cycles the month/year metric, and **g/Home** returns to the latest period.
**q**, **Esc**, and **Ctrl+C** exit either dashboard.

Source builds without release metadata identify as `v0.2.0-dev`. See
[CHANGELOG.md](CHANGELOG.md) for the pending minor-version changes.

## Archive behavior

Each archive belongs to one device. TempestKeep rejects a second device because
mixed rows would invalidate rain, wind, and temperature aggregates.

Collection requests at most five days per API call. Each chunk is committed in
one transaction. Observation inserts use the `(device_id, epoch)` key, so replay
does not create duplicate rows. Open-ended collection stores a cursor after each
committed chunk and resumes from that cursor after interruption.

The default `tempestkeep collect` run creates a timestamped backup after a
successful checkpoint. A backup is first copied to a private temporary file and
then linked into place without overwriting an existing snapshot. Set
`--backup-keep` from 0 through 365. A value of 0 disables backups.

Keep the active database on a local filesystem. Stop writers before copying the
database. Move a completed backup or export between machines. Do not place an
active WAL database in a cloud-synchronized folder.

The archive stores SI units. The CLI and MCP display layers convert values when
they promise US units. Calendar summaries use the process timezone. Set `TZ` to
the station's IANA timezone before running calendar reports on a host with a
different timezone.

## Security and privacy

Treat these files as sensitive:

- `.env` and access tokens;
- the SQLite archive, WAL, and shared-memory files;
- backups and exports; and
- terminal output that lists station, device, coordinate, or serial data.

These paths are ignored by Git. The application requests owner-only permissions
where the platform supports them. See [SECURITY.md](SECURITY.md) for private
reporting guidance and [docs/threat-model.md](docs/threat-model.md) for trust
boundaries, controls, and residual risks.

## Verification

Run the repository checks with Go 1.27.0:

```sh
make fmtcheck       # gofmt and goimports
make docs-check     # Markdown format and local links
make tidy-check     # go.mod and go.sum drift
make vet
make test           # pure-Go tests
make demo-smoke     # real MCP demo against synthetic data
make race
make fuzz           # bounded fuzz smoke tests
make lint
make workflows       # GitHub Actions syntax and semantics
make generated      # public API snapshot
make vuln
make licenses
make sbom
make secrets        # tracked files and Git history
make build-pure
make build-arm64
```

`make verify` runs the full set. Tests use finite timeouts. HTTP and MCP end-to-
end tests use local deterministic servers and do not need a token. A live smoke
test needs an explicit test token in the process environment:

```sh
# Load TEMPEST_TOKEN from a private secret manager first.
make live-smoke
unset TEMPEST_TOKEN
```

The command lists devices, fetches current conditions and a forecast, collects
one bounded history range, reads the temporary archive without a token, and then
deletes the archive. It discards API output. Do not run it with a production
token.

CI runs on pushes to `main`, pull requests, and manual dispatch. It uses Ubuntu
runners and cancels obsolete runs. Demo recording and release qualification are
manual. Release qualification builds a snapshot and does not publish it.

Use the [documentation index](docs/README.md) to find command guides, design
records, security evidence, support policy, and release procedures. See
[CONTRIBUTING.md](CONTRIBUTING.md) before proposing a change.

## Verification scope

- The repository checks do not include a production load test or formal
  verification.
- No Antithesis run has been launched or recorded.
- Live behavior depends on the WeatherFlow service and the permissions of the
  supplied token.
- CodeQL, dependency review, and some repository security features can require
  GitHub Advanced Security for a private repository.

## Contribute and report problems

Use [GitHub Issues][issues] for bugs, feature requests, and public discussion.
English reports and contributions are welcome. See [SUPPORT.md](SUPPORT.md)
for useful diagnostics and [CONTRIBUTING.md](CONTRIBUTING.md) for the pull
request process, coding rules, and required tests. Report vulnerabilities
privately using [SECURITY.md](SECURITY.md).

The OpenSSF badge above displays the live status of the existing project entry.
The [OpenSSF evidence guide](docs/openssf.md) maps the Passing criteria to the
repository's controls and records the assessment date and limits.

## Affiliation

TempestKeep is an independent project. It is not affiliated with, endorsed by,
or sponsored by WeatherFlow or the Tempest weather platform. WeatherFlow and
Tempest names and marks belong to their respective owners. See [NOTICE.md](NOTICE.md).

## License

TempestKeep uses the MIT License. See [LICENSE](LICENSE).

[version-badge]: https://img.shields.io/badge/development-v0.2.0--dev-blue
[ci-b]: https://img.shields.io/github/actions/workflow/status/lennrt/tempestkeep/ci.yml?branch=main
[ci]: https://github.com/lennrt/tempestkeep/actions/workflows/ci.yml
[license-badge]: https://img.shields.io/github/license/lennrt/tempestkeep
[go-badge]: https://img.shields.io/github/go-mod/go-version/lennrt/tempestkeep
[go-install]: https://go.dev/doc/install
[docs-badge]: https://img.shields.io/badge/Go-reference-00ADD8?logo=go&logoColor=white
[go-docs]: https://pkg.go.dev/github.com/lennrt/tempestkeep
[stars-badge]: https://img.shields.io/github/stars/lennrt/tempestkeep?style=flat
[stars]: https://github.com/lennrt/tempestkeep/stargazers
[best-practices-badge]: https://www.bestpractices.dev/projects/14460/badge
[best-practices]: https://www.bestpractices.dev/en/projects/14460/passing
[issues]: https://github.com/lennrt/tempestkeep/issues

[mcp-demo]: docs/agent.gif
[mcp-video]: https://raw.githubusercontent.com/lennrt/tempestkeep/main/docs/agent.mp4
[vhs]: https://github.com/charmbracelet/vhs
