# ADR 0004: Improve retries and command lifecycle

## Status

Accepted for development before the first stable release.

## Context

The proposed improvement patch adds retry jitter, timeout classification,
signal handling, reusable configuration, and an injectable MCP transport.
It also proposes bearer-header authentication and exposing MCP transport
errors. Those changes need separate compatibility and diagnostic review.

## Decision

Add bounded jitter to retry delays and retry HTTP 408. Preserve the existing
MaxWait cap, including when a Retry-After hint exceeds it. Sanitized API
transport errors retain timeout classification without retaining their cause.

Keep the documented WeatherFlow token query parameter. The current official
[quick start][authentication] and REST reference document query authentication;
they do not establish bearer-header support for these endpoints. Defer that
wire change until authenticated endpoint testing establishes compatibility.

Use SIGINT and SIGTERM cancellation for mcp, collect, now, and explore.
Deferred archive cleanup remains on their existing shutdown paths.
Separate CLI dispatch from process exit without changing the 0/1/2 contract.
Command handlers keep ownership of their existing output streams.

Add config.ParseBool, config.DefaultCacheTTL, and APISettings.ClientOptions.
Invalid boolean settings return ErrInvalidConfig without echoing the value.
ClientOptions performs no I/O; api.New validates the resulting options.

Allow MCP callers to supply a client and transport. A nil transport uses stdio;
the existing token-based construction remains available for internal callers.
Keep transport failures sanitized: a custom transport or malformed protocol
input can put sensitive details into an error. Context cancellation and
deadline errors remain classifiable.

When live observations fail, retain archive limitations and add an explicit
fallback note to both the dashboard and its existing JSON note field.

## Compatibility

The new configuration declarations are additive. No archive schema or MCP
tool schema changes. The fallback note and unknown-command hint wording may
change. Authentication and redirect policy retain their existing contracts.

## Validation

Test retry bounds, HTTP 408 recovery and exhaustion, timeout redaction,
configuration behavior, CLI status and streams, signal shutdown, MCP tool
registration and cancellation, and archive fallback output. Update the public
API snapshot and run make verify before committing.

[authentication]: https://apidocs.tempestwx.com/reference/quick-start
