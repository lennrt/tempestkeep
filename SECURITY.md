# Security policy

Security fixes apply to the default branch. A release's notes must identify any
additional supported release branches.

## Report a vulnerability

Use [Report a vulnerability][private-report] to send a private GitHub report over
HTTPS. Only the reporter and authorized security-advisory collaborators can see
the draft. Do not open a public issue with exploit details or sensitive data.
If GitHub reporting is unavailable, email the maintainer at
[lrudolph@hmc.edu](mailto:lrudolph@hmc.edu) to arrange a private channel before
sending exploit details. Ordinary email is not an end-to-end encrypted channel.

Include:

- the affected revision;
- prerequisites and exact reproduction steps;
- the expected and observed result;
- the impact and affected assets; and
- a suggested fix, if known.

Do not include a live token, station coordinate, station or device identifier,
serial number, archive, backup, export, or raw API response. Use synthetic values.

## Triage and remediation

The maintainer reviews reports at least weekly and acknowledges vulnerability
reports within 14 days. If no acknowledgment arrives, follow up through the
private report or the maintainer address above.

Prioritize confirmed critical vulnerabilities immediately. Aim to provide a fix
or effective mitigation within seven days; communicate progress privately if
more time is needed. Fix confirmed medium-or-higher exploitable findings from
static or dynamic analysis promptly and before the next affected release.
No publicly known medium-or-higher vulnerability may remain unpatched for more
than 60 days. Track receipt, acknowledgment, severity, affected revisions,
mitigations, and fix availability in the advisory.

Coordinate disclosure with the reporter. Publish a security advisory and a
human-readable release-note entry for security fixes, including assigned CVE or
GHSA identifiers, affected versions, impact, and upgrade or mitigation steps.
These are maintenance targets, not a commercial support guarantee. If the
project cannot meet them, disclose the limitation and update its badge answers.

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

[private-report]: https://github.com/lennrt/tempestkeep/security/advisories/new
