# MCP service

## Purpose

Expose live weather and a local archive to MCP clients through typed tools,
resources, and prompts that match the configured capabilities.

## Requirements

### Requirement: Capability-based tool registration

The server MUST expose live tools only with a live client.
It MUST expose archive tools only with an archive and write tools only with a writer.

#### Scenario: An existing archive opens without a token

- **WHEN** the server starts with an archive, read-only mode, and no live client
- **THEN** it exposes archive reads and status without live or write tools

### Requirement: Protocol-only stdout

The MCP command MUST reserve stdout for JSON-RPC and send diagnostics to stderr.

#### Scenario: The server reports a timezone warning

- **WHEN** the configured process timezone differs from the station timezone
- **THEN** the warning appears on stderr without corrupting the MCP stream

### Requirement: Inclusive calendar date inputs

Archive read tools MUST interpret explicit date-only end values as inclusive
calendar dates in the process timezone, then query through the next midnight.

#### Scenario: A caller requests one calendar day

- **WHEN** the start and end inputs name the same date
- **THEN** results include that date and exclude the next midnight

### Requirement: Exclusive backfill end dates

The backfill_archive tool MUST treat an explicit end date as an exclusive
boundary and MUST reject a start that is not before that boundary.

#### Scenario: A caller gives identical backfill start and end dates

- **WHEN** the date inputs describe an empty backfill range
- **THEN** the tool rejects the range without collecting observations
