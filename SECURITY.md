# Security policy

Security fixes apply to the default branch. No response-time or remediation
service level is promised.

## Report a vulnerability

Use the repository's private GitHub Security Advisory form. Do not open a public
issue with exploit details or sensitive data.

Include:

- the affected revision;
- prerequisites and exact reproduction steps;
- the expected and observed result;
- the impact and affected assets; and
- a suggested fix, if known.

Do not include a live token, station coordinate, station or device identifier,
serial number, archive, backup, export, or raw API response. Use synthetic values.

## Exposed credentials

If a token appears in chat, a terminal transcript, a command line, a file, or a
log, treat it as compromised. Rotate it through WeatherFlow. Remove the exposed
copy from local files and shell history. Do not rely on redaction after exposure.

## Local data

The `.env` file, SQLite archive, WAL and shared-memory files, backups, exports,
and station-list output are sensitive local data. TempestKeep ignores common
paths in Git and requests owner-only permissions where supported. The user still
owns host access controls, backups, MCP client configuration, and token rotation.

Read [docs/threat-model.md](docs/threat-model.md) for trust boundaries, controls,
residual risks, and verification commands.
