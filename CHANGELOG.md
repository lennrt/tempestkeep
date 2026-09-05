# Changelog

## 0.2.0 (unreleased)

### Added

- Vertical scrolling in `now` and `explore`: Up/Down or k/j for a row, and
  Page Up/Page Down for a page. A position hint appears only when needed.
- Enter to refresh or retry an explorer view without overlapping requests.
- README badges for development version, CI, license, Go version, Go Reference,
  and stars, plus a compact terminal-controls guide.
- An embedded development version, shared with Makefile fallback builds.

### Fixed

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
Existing navigation shortcuts and the weather artwork are retained. The plugin
manifest is prepared for version 0.2.0; no release tag or binary is published by
these changes. Release qualification still follows [RELEASING.md](RELEASING.md).
