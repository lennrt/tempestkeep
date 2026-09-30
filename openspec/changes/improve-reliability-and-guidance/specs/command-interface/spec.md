## Purpose

Define command-line validation, truthful terminal displays, station identity,
and safe configuration output for TempestKeep operators on supported platforms.

## ADDED Requirements

### Requirement: Validate command arguments before external work

Commands MUST reject unexpected positional arguments and invalid collection dates
before network or archive work. Refresh intervals MUST fit in time.Duration.

#### Scenario: An option-only command receives a filename

- **WHEN** the command parses its arguments
- **THEN** it reports usage status 2 instead of silently ignoring the filename

#### Scenario: A refresh interval exceeds the duration range

- **WHEN** the dashboard parses the interval
- **THEN** it rejects the input before starting the refresh loop

### Requirement: Safe terminal text and generated commands

Plain device listings MUST normalize API-controlled terminal labels and report
output failures. Generated Windows commands MUST quote paths as PowerShell literals.

#### Scenario: An API label includes terminal control characters

- **WHEN** list-devices prints the label
- **THEN** those characters cannot execute terminal controls

#### Scenario: A Windows path includes an apostrophe or dollar sign

- **WHEN** setup prints a command using that path
- **THEN** the command preserves the literal path without variable expansion

### Requirement: Accurate archive fallback identity

The dashboard MUST attach a discovered live station name to archived readings
only when the archive's device belongs to that station.

#### Scenario: Live weather fails and the fallback archive belongs to another station

- **WHEN** the dashboard displays the archived reading
- **THEN** it does not label it with the unrelated live station's name

### Requirement: Calendar and sensor-aware displays

Displays MUST count local calendar days across daylight-saving transitions.
They MUST preserve independent sensor readings when temperature is absent,
distinguish empty ranges from measured zeroes, and label only today's forecast as Today.

#### Scenario: March includes a spring daylight-saving transition

- **WHEN** the explorer calculates monthly coverage
- **THEN** it uses all 31 calendar days

#### Scenario: A local day starts inside a UTC-aligned chart bucket

- **WHEN** the explorer plots that day
- **THEN** it retains both partial edge buckets for the requested local date

#### Scenario: A daily row has rain and wind but no temperature

- **WHEN** the explorer renders the row
- **THEN** the known rain and wind values remain visible

### Requirement: Setup writes only parseable configuration

Setup MUST validate the complete generated dotenv content before replacing an
existing file. A validation failure MUST preserve the original bytes.

#### Scenario: Adding settings would exceed the dotenv size limit

- **WHEN** setup prepares the replacement file
- **THEN** it reports the validation failure and keeps the existing file intact
