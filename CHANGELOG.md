# Changelog

## 0.2.0 (unreleased)

### Added

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

No archive-schema, public-package API, JSON, or MCP changes. No new dependencies.
Proxies used with the default client must provide a direct endpoint with modern
TLS; redirects, old protocols, CBC-only servers, and undersized RSA or EC
certificate keys are now rejected.
Existing navigation shortcuts and the weather artwork are retained. The plugin
manifest is prepared for version 0.2.0; no release tag or binary is published by
these changes. Release qualification still follows [RELEASING.md](RELEASING.md).
