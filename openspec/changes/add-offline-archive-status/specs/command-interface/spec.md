## ADDED Requirements

### Requirement: Offline archive status

The status command MUST read an existing archive without token, network, or write
access. It MUST report count, nullable first/last timestamps and age, freshness,
saved completion, and up to ten largest internal gaps exceeding one hour.
It MUST state that stored observations do not establish continuous coverage.

#### Scenario: An empty archive is inspected as JSON

- **WHEN** the archive contains no observations
- **THEN** count is zero, timestamps and age are null, and gaps is an empty array

#### Scenario: A collector needs repair boundaries

- **WHEN** observations contain an internal gap greater than one hour
- **THEN** status reports the boundary epochs without changing rows or metadata

#### Scenario: The archive path does not exist

- **WHEN** status opens that path
- **THEN** it fails without creating the file or its parent directory

#### Scenario: Invalid format precedes invalid configuration

- **WHEN** the format is neither text nor json
- **THEN** status returns usage status 2 before reading configuration or the archive

#### Scenario: Stored completion metadata is invalid

- **WHEN** the completion marker is neither 0 nor 1
- **THEN** status returns the classified invalid-checkpoint error

#### Scenario: The newest row is in the future

- **WHEN** its age is negative
- **THEN** status reports that age and does not label the archive fresh

### Requirement: Bounded stored metadata reads

Metadata readers MUST enforce the writer's existing byte and NUL limits before
returning values. Oversized values MUST be excluded by SQL before Go allocation.
Malformed stored values MUST retain ErrInvalidArchive classification.

#### Scenario: An external application stores oversized metadata

- **WHEN** either archive metadata reader encounters more than 4,096 bytes
- **THEN** it returns ErrInvalidArchive without loading those bytes as a Go string

#### Scenario: A valid multibyte metadata value reaches the boundary

- **WHEN** the value is exactly 4,096 bytes without NUL
- **THEN** both metadata readers return the exact value
