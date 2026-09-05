# ADR 0003: Make the OpenSSF security baseline explicit

## Status

Accepted for development before the first stable release.

## Context

The OpenSSF Passing assessment needs evidence of implemented practices. Existing
tests, static analysis, and security documentation cover much of the baseline.
The default HTTP client nevertheless inherits mutable global transport settings
and follows redirects. Its token is carried in an API query parameter.

CI also downloaded a golangci-lint binary built with Go 1.26 while this project
requires Go 1.27. Scorecard wrote to a nonexistent workspace subdirectory.

## Decision

Give the default API client a shared private transport. Require TLS 1.2 or newer,
ephemeral key agreement, and authenticated encryption. Preserve ordinary chain
and hostname verification, then enforce RSA certificate keys of at least 2048
bits and elliptic-curve certificate keys of at least 224 bits. Use Go's TLS,
certificate validation, and cryptographic randomness implementations.

Do not follow redirects with the default client. Return the existing typed
HTTP error for 3xx responses without contacting the redirect destination.
Keep explicit test endpoints and borrowed HTTP clients available; callers must
trust their transport and preserve the security policy for real credentials.

Build the pinned linter through `make lint` with the required Go toolchain. Keep
Scorecard output in its workspace and retain it as a workflow artifact. An
imports-tool failure must fail `make fmtcheck`, even if it prints no stdout.

Document issue triage, vulnerability response, release-note requirements, and
developer review practices. Store justified Passing answers in
`.bestpractices.json` for the existing project entry 14460. Display that entry's
live badge; a file of proposed answers does not itself award a badge.

## Compatibility

No exported Go declaration, JSON field, MCP schema, or archive schema changes.
The default client now rejects redirects and legacy TLS configurations. A proxy
that previously needed either must provide a direct endpoint with modern TLS.
The explicit `WithHTTPClient` and `WithBaseURL` options retain their contracts.

No release is published or tagged by this change. Human-readable release notes
and immutable SemVer tags remain requirements for a future approved release.

## Validation

Local TLS handshakes test accepted protocols and rejection of old protocols,
CBC-only servers, and untrusted certificates. Redirect tests ensure the target
receives no request. Certificate-policy tests cover weak issuers and alternate
verified chains. Run `make verify` and inspect CI, CodeQL, and Scorecard results
for the pushed commit before claiming that their checks passed.
