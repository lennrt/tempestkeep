## ADDED Requirements

### Requirement: MCP device configuration precedence

The MCP command MUST accept a positive `--device-id` and otherwise read
`TEMPEST_DEVICE_ID`. An explicit flag MUST override the environment. Empty or zero
environment values MUST retain automatic discovery. Invalid flags MUST fail
before dotenv access; invalid resolved inputs MUST fail before archive/API work.

#### Scenario: The flag overrides an invalid environment value

- **WHEN** the command receives a positive explicit device ID
- **THEN** it uses that ID without parsing the overridden environment value

#### Scenario: An invalid device flag is supplied

- **WHEN** the flag is zero, negative, malformed, or overflowing
- **THEN** startup reports usage failure without reading configuration or opening an archive

### Requirement: Early report epoch and span validation

Stats and export MUST reject date ranges outside supported observation epochs or above
the store span limit before dotenv or archive access. Invalid explicit ranges
MUST produce usage status 2. Valid inclusive local dates and DST boundaries MUST
retain their existing meaning.

#### Scenario: A pre-epoch date names a missing archive

- **WHEN** stats or export receives that range
- **THEN** it reports invalid dates rather than a missing-archive runtime error

#### Scenario: A range exceeds the supported span

- **WHEN** stats or export resolves its date bounds
- **THEN** it rejects the range before configuration and filesystem work

#### Scenario: A valid range spans a daylight-saving transition

- **WHEN** the command resolves its inclusive local dates
- **THEN** the result contains the correct 23-hour or 25-hour day
