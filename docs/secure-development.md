# Secure development

Read this guide with the [threat model](threat-model.md) and
[security policy](../SECURITY.md). On 2026-09-05, the maintainer, Lennart Rudolph,
confirmed familiarity with secure-design principles and relevant vulnerability
classes for the OpenSSF assessment.

## Design review

| Principle | Apply it in TempestKeep |
|---|---|
| Keep mechanisms simple | Share a bounded API client and separate collection from storage queries. Minimize execution and configuration paths. |
| Deny unsafe defaults | Reject malformed configuration, invalid observations, insecure files, and unsupported queries. Never turn a failed check into success. |
| Check every operation | Validate every MCP request and store operation. Enforce read-only access in SQLite as well as the query validator. |
| Make the design public | Publish code and threat assumptions. Protect tokens instead of relying on hidden implementation details. |
| Require independent protections | Combine input checks with database restrictions and normal TLS verification with certificate-key limits. |
| Minimize privileges | Use read-only handles, optional MCP write capabilities, private data files, and scoped CI permissions. |
| Avoid unnecessary shared state | Bind one device to an archive, keep temporary files private, and copy retained caller-owned values. |
| Make safe use understandable | Document sensitive outputs, defaults, errors, and recovery. Keep credentials out of command arguments. |
| Reduce exposed interfaces | Use MCP stdio, fixed tool registrations, bounded API calls, and no arbitrary write-SQL endpoint. |
| Accept only valid input | Check permitted operations, syntax, ranges, counts, sizes, and time budgets before performing work. |

These practices support the [OpenSSF secure-design criterion][criteria]. A
document does not replace developer understanding and continued review.

## Common errors and defenses

| Error class | Defense and evidence |
|---|---|
| SQL injection and unauthorized writes | Parameterized stored queries, a single-query validator, SQLite `query_only`, and write-attempt regression tests. |
| Command injection | No production execution of user-supplied shell commands; keep arguments typed and bounded. |
| Missing authorization | Register capabilities according to token, archive, and read-only settings; validate device ownership on writes. |
| Credential disclosure | Redacted diagnostics, private files, default-client redirect rejection, and scans of files and Git history. |
| Malicious paths and file races | Regular-file and identity checks, symlink rejection, and atomic backups that never overwrite existing files. |
| Resource exhaustion | Limits on bytes, rows, columns, caches, retries, queries, and collection windows; contexts and deadlines. |
| Memory and concurrency defects | Memory-safe Go, defensive copies, serialized writer operations, race checks, and fuzzing. |
| Terminal control injection | Normalize external labels before styling and test control characters, Unicode, long inputs, and frame bounds. |
| Broken transport trust | Standard-library TLS, normal certificate verification, modern algorithms, and explicit custom-transport ownership. |
| Dependency compromise | Pinned tools and Actions, Go checksums, vulnerability and license checks, and reviewed updates. |

The [property catalog](testing/properties.md) names executable regressions.
For study, use [OpenSSF's security courses][course] and [CWE Top 25][cwe].
Report confirmed findings through [SECURITY.md](../SECURITY.md).

## Cryptographic behavior

The default WeatherFlow endpoint uses HTTPS. `pkg/tempest/api/transport.go`
requires TLS 1.2 or newer. TLS 1.2 permits ECDHE with AES-GCM or
ChaCha20-Poly1305; TLS 1.3 uses Go's authenticated-encryption suites. Session
encryption uses at least 128-bit keys and SHA-256 or stronger hashes. Go's
default supported groups include X25519 and P-256 or stronger curves, with
ephemeral key agreement for forward secrecy.

Normal certificate-chain and hostname validation stays enabled. At least one
verified chain must also satisfy RSA keys of at least 2048 bits and EC keys of
at least 224 bits. Go validates other supported certificate algorithms,
including Ed25519. TLS nonces and private key material use Go's cryptographic
randomness. TempestKeep implements no cipher, key exchange, or random generator.

`WithHTTPClient` is an explicit trust boundary. Callers must preserve equivalent
TLS, certificate, redirect, and randomness settings for real credentials.
Plain HTTP endpoints exist for local synthetic tests and demos; do not use
them with real tokens. Do not re-enable obsolete algorithms through custom
transports or runtime settings.

The `.env` token authenticates outbound API requests. It is not an inbound user
password and cannot be replaced by a password hash. Use an owner-only file or
a trusted process environment. Use host or disk encryption for protection at
rest when needed.

Source and approved release delivery use HTTPS. Never validate a download using
a checksum obtained over unauthenticated HTTP. See [RELEASING.md](../RELEASING.md).

[criteria]: https://www.bestpractices.dev/en/criteria/0#know_secure_design
[course]: https://openssf.org/training/courses/
[cwe]: https://cwe.mitre.org/top25/
