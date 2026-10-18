# Changelog

## 0.4.0 (proposed, unreleased)

### Added

- Explicit CLI archive repair with paired inclusive `collect --start` and
  `--end` dates, bounded to 366 calendar days and safe to replay.
- Exact `before_epoch` continuation for MCP backfill, including batches that
  stop inside a daylight-saving transition day.
- `rain_readings` and `lightning_readings` counts in observation-series buckets,
  with missing-value displays and incomplete-coverage notices in MCP Apps.
- Pointer, touch, and keyboard inspection for every returned chart point,
  including dense series above 60 readings.
- Optional `export --metadata` fields for device ID, report interval, wind
  interval, precipitation type, and source. Defaults remain unchanged.
- Read-only streaming access to stored observation metadata through
  `store.ArchiveObs` and `Store.EachArchiveObs`.
- MCP `--device-id` and `TEMPEST_DEVICE_ID` selection for collection and the
  default live station, with lazy resolution and explicit station overrides.
- A searchable Devices table in the `list_stations` app, with parent stations,
  exact identifiers, bounded traversal, and compatible refresh state.
- Observation and rain-reading counts in rain summaries, with missing/partial
  coverage guidance in CLI reports, MCP results, and the rainfall app view.

### Changed

- Preserve table search, sorting, dataset selection, and valid page position
  across compatible refreshes. New requests and changed table structures reset
  controls. Focused search text and selection survive a pending refresh.
- Center both time and temperature by calendar month in the trend fit, so
  uneven seasonal coverage does not dilute the within-month slope.
- Report zero slope for repeated constant monthly temperatures and omit the
  undefined R². See [ADR 0009](docs/adr/0009-seasonal-trend-estimation.md).
- Bump active development, plugin, and embedded app metadata to 0.4.0.
- Choose the nearest eligible pressure sample on either side of the requested
  window start. Sparse history can now use a pair spanning at least half the
  window, and an older sample can no longer mask a closer reading after it.
- Reject unsupported CLI report and export dates before configuration or archive
  access, with usage status 2 and unchanged local calendar boundaries.
- Return a null rain summary total when no rain readings exist. Preserve measured
  zero and sum only reported values in partially observed ranges.
- Lead the docs overview with multi-station workflows and guides to interactive
  charts, table refresh, archive repair, metadata exports, and offline analysis.

### Compatibility

Archive recovery adds a `before_epoch` input to `backfill_archive` and two
reading-count fields to `get_observations` points. Existing numeric totals
keep their types; a zero count means its total is unknown, and a count below
`obs` means partial coverage. Exported `SeriesPoint` gains the corresponding
fields. See [ADR 0008](docs/adr/0008-archive-recovery-and-coverage.md).

Default CSV columns and JSON Lines fields remain unchanged. `--metadata` appends
five CSV columns and optional JSON fields. Metadata retains its stored units in
SI and US exports. Older schemas expose unavailable metadata as missing. The
store API additions do not change `model.Obs`. See
[ADR 0010](docs/adr/0010-export-metadata-and-development-version.md).

MCP now honors `TEMPEST_DEVICE_ID`; empty or zero keeps automatic selection.
An invalid or inaccessible selection does not fall back to another live device.
Sparse pressure results can change because comparison samples now match the
documented nearest-reading rule. See
[ADR 0011](docs/adr/0011-mcp-device-selection-and-pressure-samples.md).

`RainStats.TotalIn` changes from `float64` to `*float64`; Go callers must check
for nil. CLI rain JSON and MCP `rain_stats` now always encode `total_in` as a
number or `null`, with `observations` and `rain_readings` counts. Consumers must
accept null and the additional fields. Day counts, thresholds, spell rules,
and other rain aggregate types retain their contracts. Counts describe samples,
not continuous coverage. See [ADR 0012](docs/adr/0012-rain-summary-coverage.md).

Existing archives need no migration. Version 0.4.0 remains a development target;
no tag, release, plugin publication, or merge is part of this change.
Qualification still follows [RELEASING.md](RELEASING.md).

## 0.3.0 (proposed, unreleased)

### Added

- MCP Apps views for available read-only weather and archive tools, embedded in
  the Go binary and rendered by supporting MCP hosts.
- Weather charts and searchable, sortable, paginated tables with host themes
  and refresh through the same read-only tool.
- Self-contained app resources with no external network origins or device
  permissions. API credentials remain in the Go server.
- Protocol and Playwright browser coverage, plus reproducible screenshots from
  a synthetic MCP Apps test host.
- MCP Apps setup, compatibility, privacy, architecture, and OpenSpec requirements.

### Changed

- Pin Go 1.27.2 for module builds, repository checks, CI, and release qualification.
  Contributors must use the updated patch toolchain. See
  [ADR 0007](docs/adr/0007-go-patch-toolchain.md).
- Update the pinned golangci-lint to v2.14.0 so static analysis can read the
  patched toolchain's compiler export data.

### Compatibility

MCP Apps is additive. Hosts that advertise support receive UI resource metadata
on read-only tools. Ordinary clients retain the existing tools and results.
Resource discovery gains HTML resources. Existing tool schemas, annotations,
result shapes, archive formats, and configuration remain unchanged.

The app's refresh action does not collect history or mutate the archive.
Frontend development uses pinned npm dependencies. The Go build and runtime do
not require Node.js. Existing archives need no migration. See
[ADR 0006](docs/adr/0006-mcp-apps.md) for the protocol and security boundaries.

Version 0.3.0 is proposed because MCP Apps adds a user-facing feature. Development
and plugin metadata use that version. No tag, release, or plugin publication is
part of this change. Qualification still follows [RELEASING.md](RELEASING.md).

## 0.2.0 (unreleased)

### Added

- OpenSpec baseline requirements, a tracked reliability proposal, and strict
  specification checks through a pinned development-only npm dependency.
- Native macOS and Windows CI tests alongside the existing Linux checks.
- Worked MCP and SQL queries, recovery instructions, and explicit guidance on
  date boundaries, missing readings, report intervals, and read-only capabilities.
- Signal-aware shutdown: `mcp`, `collect`, `now`, and `explore` stop cleanly
  on SIGTERM as well as SIGINT, so MCP clients, cron, and service managers
  that terminate the process no longer leave the archive without a clean
  close. `collect` still saves its resume cursor on either signal.
- `config.ParseBool`, `config.DefaultCacheTTL`, and
  `config.APISettings.ClientOptions` so every command builds its API client
  the same way. `config.FirstNonEmpty` is now a thin wrapper over `cmp.Or`.
- `mcpapp.Options.Client` and `mcpapp.Options.Transport` so the MCP server
  can be driven in-process over `mcp.NewInMemoryTransports` in tests.
- OpenSSF Best Practices badge and criterion evidence for project 14460.
- Documented contribution, security triage, and release-note requirements.
- Vertical scrolling in `now` and `explore`: Up/Down or k/j for a row, and
  Page Up/Page Down for a page. A position hint appears only when needed.
- Enter to refresh or retry an explorer view without overlapping requests.
- README badges for development version, CI, license, Go version, Go Reference,
  and stars, plus a compact terminal-controls guide.
- An embedded development version, shared with Makefile fallback builds.

### Changed

- MCP archive writes serialize within one server. A queued call can be canceled.
  Small backfill budgets persist empty-history progress across calls and restarts.
- Documentation and plugin instructions use a SimpleEnglish Plain review, with
  commands, identifiers, units, and technical meaning preserved.
- The existing Windows go-localereader dependency uses its upstream license-only
  follow-up commit. Its Go source is unchanged, and license scanning now succeeds.
- Retries use jittered exponential backoff (bounded by `RetryPolicy` and
  respecting `Retry-After` up to the configured maximum) and also retry HTTP 408.
- Sanitized transport errors keep their timeout classification: they still
  match `api.ErrTransport`, and a deadline additionally matches
  `context.DeadlineExceeded` and reports `Timeout() == true`.
- The `tempestkeep` entry point is a testable `run` function with named exit
  statuses; helpers no longer call `os.Exit`.
- `tempestkeep now` marks a frame that fell back to the archive because live
  data was unavailable, instead of presenting the fallback silently.

### Fixed

- Reject busy or incomplete SQLite checkpoints before creating a file backup.
- Preserve seed intent and terminal cursors after interrupted collection.
  Reject out-of-window observations before they can advance an archive watermark.
- Preserve successful empty windows across bounded MCP sync calls, so repeated
  calls can reach newer observations after an outage longer than one work budget.
- Reject unsuccessful API status envelopes, missing result collections, mismatched
  devices, and unsupported observation layouts. Cache only accepted responses.
- Bound nested API arrays before typed decoding and prevent Retry-After overflow.
- Bound SQL values and columns inside SQLite, restore connection limits after
  failure or cancellation, and reject numbers that JSON cannot encode.
- Count actual epoch-aligned series buckets before enforcing point limits.
- Keep forecast station labels, open-ended summary dates, and archive freshness
  consistent with the selected station, requested range, and current time.
- Match device identity before combining live station labels or pressure trends
  with archive data. Omit stale pressure trends from current conditions and
  normalize the three-hour pressure field to its stated interval.
- Enforce max_days as a hard backfill cap. Validate collection dates and reject
  unexpected CLI arguments before network or archive work.
- Normalize device-list terminal text, propagate output failures, and quote
  generated PowerShell commands as literal paths.
- Match forecast Today labels to actual dates and retain day-chart edge buckets
  for timezones whose offset is not a whole hour.
- Count calendar days across daylight-saving changes. Retain rain and gusts when
  temperature is absent, and distinguish empty ranges from measured calm weather.
- Validate complete setup output before replacing a dotenv file, preserving the
  original file when the generated settings would exceed parser limits.
- Assign fractional wind headings to the correct compass sector. Include all
  known wind speeds in calm percentages while excluding unknown headings from sectors.
- Estimate solar energy from positive report intervals rather than extrapolating
  sparse readings across complete 15-minute buckets. Preserve sensor peaks when
  interval duration is missing.
- Break dry and storm-free spells on days without relevant sensor readings.
  Omit apparent temperatures when a required humidity or wind reading is absent.
- Honor nested documentation fences and encoded filenames. Keep checked text
  files in LF format on Windows, and omit installed dependencies from docs checks.
- Keep synthetic API observations inside their requested range and exercise the
  boundary with regression tests.
- Accept the 22-field `obs_st` rows the REST observations endpoint returns.
  The four derived rain values after the 18 sensor fields are ignored; rows
  wider than 22 fields are still rejected. Previously collection failed on the
  first chunk with `obs_st row must contain 1..18 fields`, so affected
  stations could not be archived.
- Require modern TLS and certificate key sizes with the default API client;
  return an HTTP error for redirects instead of following them. Custom HTTP
  clients remain caller-owned security boundaries.
- Build CI's pinned linter with Go 1.27, retain Scorecard results, and fail
  formatting checks when the imports tool fails.
- Update static analyzers for Go 1.27's Linux library and make rain-summary
  regression fixtures independent of the time of day.
- Bound every interactive frame to the terminal width and height, including
  loading, errors, and narrow-window notices. Keep fitting cards centered.
- Prevent long station names, conditions, status messages, and error text from
  widening cards. Normalize external labels before applying terminal styling.
- Fill the complete forecast width with partial forecasts, and safely handle
  an empty forecast strip.
- Clear old explorer data when changing view or period so it cannot appear
  under the new calendar while a request is pending. Preserve stale-response
  rejection and make failed loads retryable.
- Validate live-client configuration before opening a local archive, avoiding
  an open store left behind when client initialization fails.

### Compatibility

Public Go signatures, observation columns, tool names, and JSON field shapes stay
unchanged. Private metadata now preserves bounded MCP backfill and sync progress.
Invalid arguments and responses that previously appeared to succeed now fail.
Sparse-data solar estimates and wind percentages can change to reflect their
actual samples. OpenSpec adds development dependencies; the Go binary still
builds without Node.js. See [ADR 0005](docs/adr/0005-reliability-and-specifications.md).
Proxies used with the default client must provide a direct endpoint with modern
TLS; redirects, old protocols, CBC-only servers, and undersized RSA or EC
certificate keys are now rejected.
Existing navigation shortcuts and the weather artwork are retained. The plugin
manifest is prepared for version 0.2.0; no release tag or binary is published by
these changes. Release qualification still follows [RELEASING.md](RELEASING.md).
