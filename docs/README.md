# Documentation

Start with the [repository README](../README.md). It covers requirements,
building, setup, collection, MCP operation, privacy, and verification.

## Use TempestKeep

- [`tempest` command guide](../cmd/tempest/README.md): setup, collection,
  displays, exports, and configuration precedence.
- [`tempest-mcp` command guide](../cmd/tempest-mcp/README.md): MCP inputs,
  capabilities, limits, and startup behavior.
- [Plugin guide](../plugin/README.md): optional client integration.
- [Support guide](../SUPPORT.md): where to ask for help and what to include.

## Understand the design

- [Architecture](architecture.md): process, API, collection, storage, time, and
  unit boundaries.
- [Public API snapshot](public-api.txt): exported Go declarations and JSON tags.
- [Engineering baseline ADR](adr/0001-modern-go-engineering-baseline.md): Go,
  API, compatibility, CI, and release decisions.

## Review correctness and security

- [Correctness properties](testing/properties.md): guarantees converted into
  executable test targets.
- [Threat model](threat-model.md): assets, trust roots, controls, and residual
  risks.
- [Antithesis workload design](testing/antithesis-scratchbook.md): bounded
  workload and fault-injection plan. No run result is claimed.
- [Dependency license policy](license-policy.md): accepted licenses and
  checksum-bound exceptions.
- [Security policy](../SECURITY.md): private reporting and sensitive-data rules.

## Maintain and release

- [Contribution guide](../CONTRIBUTING.md): required checks and design rules.
- [Release process](../RELEASING.md): qualification, approval, evidence, and
  rollback requirements.

## Demo sources

The four `.tape` files use the local mock API. They do not need a station or
token.

- [CLI and MCP demo](demo.tape)
- [Setup demo](setup.tape)
- [MCP agent demo](agent.tape)
- [Archive explorer demo](explore.tape)

Run `make vhs` to rebuild all GIFs. The command requires the pinned VHS version
from the demo workflow, plus `ttyd` and `ffmpeg`.
