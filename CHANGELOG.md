# Changelog

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
