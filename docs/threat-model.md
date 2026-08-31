# Threat model

## Scope

This model covers `tempest`, `tempest-mcp`, the shared Go packages, the local
SQLite archive, configuration files, and repository automation. It does not
cover the WeatherFlow service, the MCP client, the operating system, or GitHub
as trusted implementations.

## Protected assets

- WeatherFlow access tokens.
- Station names, coordinates, timezones, serial numbers, and device IDs.
- Observation history, backups, and exports.
- Archive integrity and the one-device ownership rule.
- MCP protocol integrity on stdout.
- Source and build dependency integrity.

## Trust roots

- The user and the local operating-system account.
- The Go 1.27.0 toolchain and Go checksum database.
- The configured WeatherFlow HTTPS endpoint.
- Full commit SHAs in GitHub Actions.
- The repository's reviewed source and generated API snapshot.

## Boundaries and controls

| Boundary | Untrusted input | Main controls | Failure behavior |
|---|---|---|---|
| Environment and `.env` | Paths, token, booleans, durations, and identifiers | Size and syntax limits; private regular file; no symlink; environment wins | Reject malformed or insecure configuration |
| WeatherFlow HTTP | Status, headers, body, JSON fields, counts, and delays | HTTPS by default; per-attempt timeout; bounded retry; body and semantic limits; field validation | Reject the complete response; return a redacted typed error |
| Borrowed HTTP client | Transport and proxy behavior | Explicit borrowed ownership; token and URL omitted from returned errors | Caller must trust the transport |
| Collector | Replay, cancellation, partial progress, and concurrent calls | One operation per instance; bounded chunks; transactional insert; persisted cursor; idempotent key | Keep committed chunks; return a resume point and error |
| SQLite file | Symlink, replacement race, invalid schema, mixed devices, and hostile SQL data | Regular-file and identity checks; one-device binding; validated observations; read-only handle | Fail closed without creating a read archive |
| Read-only SQL | Arbitrary query text and large results | One statement; SELECT/CTE filter; SQLite `query_only`; time, byte, row, and column limits | Cancel or return `ErrResultTooLarge` or `ErrInvalidArgument` |
| Backup directory | Collision, unrelated files, permissive mode, and large directory | Owner-only mode; exact filename format; entry limit; atomic no-overwrite link | Preserve existing backups and return failure |
| MCP stdio | Tool arguments and untrusted archive/API values | Generated schemas; input bounds; stdout reserved for JSON-RPC; redacted stderr | Return a tool error without raw diagnostic data |
| CI and tools | Dependency updates, mutable actions, and malicious history | Full action SHAs; minimum permissions; exact Go and tool versions; checksums; secret and dependency scans | Required job fails |

## Diagnostic sinks

Logs, errors, metrics, test fixtures, screenshots, workflow artifacts, and the
public API snapshot must not contain credentials, response bodies, personal
paths, station coordinates, serial numbers, or raw correlation identifiers.
Commands that intentionally list station or device data are user-facing data
outputs, not diagnostics. Treat their output as sensitive.

## Residual risks

- The WeatherFlow API uses a token query parameter. A borrowed HTTP transport,
  local proxy, or operating-system network diagnostic can observe the full URL.
- A process with the same operating-system privileges can read process memory,
  the archive, and permitted configuration files.
- SQLite commits protect database consistency, but disk or filesystem failure
  can produce an ambiguous client result. Replay is required after uncertainty.
- A checkpointed file backup is not a substitute for tested off-machine backup
  and restore procedures.
- MCP clients can request and retain weather data. Configure only trusted
  clients and review their logging policy.
- Private-repository CodeQL and dependency review can require GitHub Advanced
  Security. A missing entitlement is an external blocker, not a passing skip.
- No penetration test, formal verification, production load test, or Antithesis
  run has been completed.

## Executable verification

```sh
make test race fuzz lint generated
make vuln licenses sbom secrets
make build-pure build-arm64
```

Before distribution, also inspect tracked, untracked, and ignored files; scan
Git history; review the SBOM and dependency licenses; and verify that release
and plugin metadata contain no private path or unsupported claim.
