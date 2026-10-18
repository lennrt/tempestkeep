## Purpose

Let archive owners replay a bounded historical period through the CLI without
changing their ordinary collection progress.

## ADDED Requirements

### Requirement: Explicit CLI range collection

The collect command MUST accept paired inclusive local `--start` and `--end`
calendar dates for at most 366 calendar days. It MUST reject missing, malformed,
reversed, unsupported, future, or excessive bounds and an explicit
`--backfill-start` conflict before discovery or archive creation. An environment
seed floor MUST NOT change an explicit range.

#### Scenario: Repair an existing gap

- **WHEN** the requested dates precede the archive watermark
- **THEN** collection requests that range and inserts available missing observations
- **AND** replay does not duplicate rows or change seed or sync progress metadata

#### Scenario: A local day spans a clock change

- **WHEN** a user requests a daylight-saving transition date
- **THEN** the range includes that entire local date and excludes the next date

#### Scenario: Repair stops after a committed chunk

- **WHEN** an API error or cancellation interrupts a range
- **THEN** committed rows remain and diagnostics instruct the user to repeat the same range

#### Scenario: Invalid input

- **WHEN** a range violates an input bound or conflicts with an explicit seed floor
- **THEN** the command fails before making an API request or opening the archive
