# Documentation

TempestKeep brings live weather, forecasts, and your local observation history
into an MCP client or terminal. Start with the
[setup and connection guide](../README.md#connect-your-agent),
then choose a workflow below. One Go executable includes the MCP server,
interactive app, terminal dashboards, reports, and exports.

## Work with several stations

If your token can access multiple stations, choose which Tempest device supplies
your archive and default live station:

1. Run `tempestkeep list-devices` or ask your agent to call `list_stations`.
   In a supporting MCP Apps host, open the **Devices** table and search by ID,
   serial number, or station name. Each device keeps its parent station beside it.
2. Set `TEMPEST_DEVICE_ID` or start MCP with `--device-id` to select that device.
   The flag overrides the environment. Without a selection, TempestKeep chooses
   the first accessible Tempest device.
3. Keep **one device per archive**. Give each device its own archive path when
   collecting from several stations; TempestKeep rejects writes for another device.

The selected device controls collection and the default station for live reads.
Conditions and forecasts remain station-level results. Pass `station_id` to
`forecast` or `station_details` to read another station without changing that
default or the archive. See [device selection and capabilities](mcp.md#capabilities),
[MCP configuration](mcp.md#build-and-connect), and the
[searchable device view](mcp-apps.md#read-charts-and-tables).

## Explore weather in your conversation

In a host that supports MCP Apps, ask for a forecast, daily summary, wind rose,
or archive query, then open its interactive view.

| Try this | What you can do |
|---|---|
| Inspect a dense weather chart | Point, tap, or use the keyboard slider to inspect every displayed reading, up to 2,000 points, with its time and units. |
| Find a particular day or device | Search returned rows, sort columns, and move through table pages. |
| Refresh while exploring | Keep the selected table, search, sort, and valid page when the refreshed columns remain compatible. |
| Check incomplete observations | Inspect rain or lightning coverage in observation series, and distinguish missing rain, measured zero, and partial totals in rainfall summaries. |

Refresh repeats a read-only tool; it does not collect new archive history.
Search covers received rows, and display limits are shown. Clients without app
support still receive ordinary MCP results. The [MCP Apps guide](mcp-apps.md)
explains controls, limits, and host support.

## Repair and reuse an archive

- **Fill a historical gap.** Use `collect --start YYYY-MM-DD --end YYYY-MM-DD`
  for an inclusive range of up to 366 local calendar days. Repeat an interrupted
  command safely: committed observations remain and duplicates are ignored.
  See [collection and backups](../cmd/tempestkeep/README.md#collection).
- **Continue a bounded MCP backfill.** Inspect `archive_status`, then use
  `backfill_archive`. When more history remains, explicit windows return
  `next_before` for exact continuation without changing the shared collection cursor. Follow the
  [archive-building workflow](mcp.md#build-an-archive).
- **Export enough context for your own analysis.** Add `export --metadata` to CSV
  or JSON Lines exports for stored device identity, report and wind intervals,
  precipitation type, and source. Report intervals let you reproduce the solar
  energy estimate. See the
  [export guide](../cmd/tempestkeep/README.md#export-observations).

## Ask questions about your weather history

Compare months, find recorded extremes, revisit a date across years, inspect
wind direction, or examine pressure changes. Start with the
[tool-to-question guide](querying.md#choose-a-query) and
[worked queries](querying.md#worked-tool-calls). For a terminal report, use
[`tempestkeep stats`](../cmd/tempestkeep/README.md#archive-reports); for a custom
archive question, use read-only `query_sql`. Coverage and missing sensor readings
remain part of interpreting any result.

An existing archive supports reports, exports, and history queries without a
token or live API connection. MCP `--read-only` removes collection tools while
keeping available reads; a configured token can still enable live weather.
See [offline startup and capabilities](mcp.md#start-and-stop) and
[dates, units, and aggregation](querying.md).

## User guides

- [`tempestkeep` command guide](../cmd/tempestkeep/README.md): setup, collection,
  displays, exports, and configuration precedence.
- [`tempestkeep mcp` guide](mcp.md): MCP inputs,
  capabilities, limits, and startup behavior.
- [MCP Apps guide](mcp-apps.md): interactive views, host compatibility, privacy,
  frontend development, and synthetic screenshot reproduction.
- [Query guide](querying.md): working tool and SQL examples, units, timezones,
  aggregation, and missing observations.
- [Troubleshooting](troubleshooting.md): connection, configuration, collection,
  backup, and report problems.
- [Plugin guide](../plugin/README.md): optional client integration.
- [Support guide](../SUPPORT.md): where to ask for help and what to include.

## Understand the design

- [Architecture](architecture.md): process, API, collection, storage, time, and
  unit boundaries.
- [Public API snapshot](public-api.txt): exported Go declarations and JSON tags.
- [Engineering baseline ADR](adr/0001-modern-go-engineering-baseline.md): Go,
  API, compatibility, CI, and release decisions.
- [Single-command and Homebrew tap ADR](adr/0002-single-command-and-homebrew-tap.md):
  executable naming, MCP dispatch, and distribution decisions.
- [Retry and command lifecycle ADR](adr/0004-retry-and-command-lifecycle.md):
  retry classification, configuration, shutdown, and MCP input decisions.
- [OpenSSF baseline ADR](adr/0003-openssf-security-baseline.md): transport
  defaults, CI repairs, and evidence requirements.
- [Reliability and specification ADR](adr/0005-reliability-and-specifications.md):
  checkpoint, API, query, backfill, and specification workflow decisions.

- [MCP Apps ADR](adr/0006-mcp-apps.md): embedded views, protocol compatibility,
  browser boundaries, and the proposed 0.3.0 release.

- [Go patch toolchain ADR](adr/0007-go-patch-toolchain.md): the Go 1.27.2 pin and
  qualification of the updated toolchain.

- [Archive recovery and coverage ADR](adr/0008-archive-recovery-and-coverage.md):
  explicit collection ranges, exact MCP continuation, and event sensor counts.

- [Seasonal trend ADR](adr/0009-seasonal-trend-estimation.md): a common trend
  with separate calendar-month baselines and explicit limits on interpretation.

- [Export metadata and version ADR](adr/0010-export-metadata-and-development-version.md):
  opt-in observation metadata, streaming access, and the 0.4.0 development target.
- [Device selection and pressure ADR](adr/0011-mcp-device-selection-and-pressure-samples.md):
  MCP device choice, station-device tables, pressure samples, and report bounds.
- [Rain summary coverage ADR](adr/0012-rain-summary-coverage.md): nullable totals,
  sample counts, and missing/partial rain in reports and interactive views.

## Review correctness and security

- [Correctness properties](testing/properties.md): guarantees converted into
  executable test targets.
- [Threat model](threat-model.md): assets, trust roots, controls, and residual
  risks.
- [Antithesis workload design](testing/antithesis-scratchbook.md): bounded
  workload and fault-injection plan. No run result is claimed.
- [Dependency license policy](license-policy.md): accepted licenses and
  checksum-bound exceptions.
- [Security policy](../SECURITY.md): private reporting and sensitive-data rules.
- [Secure development](secure-development.md): review principles, common
  vulnerability classes, and cryptographic defaults.
- [OpenSSF assessment](openssf.md): Passing criteria evidence and maintenance.

## Maintain and release

- [Contribution guide](../CONTRIBUTING.md): required checks and design rules.
- [OpenSpec workflow](openspec.md): proposals, specifications, and change review.
- [Release process](../RELEASING.md): qualification, approval, evidence, and
  rollback requirements.
- [Homebrew tap runbook](homebrew.md): formula contract, validation, and
  project-maintained tap publication.

## Demo sources

The four `.tape` files use the local mock API. They do not need a station or
token.

- [MCP recording guide](demo.md): replay, smoke check, and record the tool demo.
- [MCP Apps screenshots](mcp-apps.md#reproduce-the-screenshots): capture the
  embedded app with Playwright and synthetic data.
- [CLI and MCP demo](demo.tape)
- [Setup demo](setup.tape)
- [MCP agent demo](agent.tape)
- [Archive explorer demo](explore.tape)

Run `make vhs` to rebuild all GIFs. The command requires the pinned VHS version
from the demo workflow, plus `ttyd` and `ffmpeg`.
