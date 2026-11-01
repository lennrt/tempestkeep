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

Metadata readers MUST enforce the writer's existing UTF-8 byte and NUL limits
before returning values. SQL MUST exclude values over 8,192 encoded bytes before
Go allocation. After UTF-8 conversion, readers MUST reject values over 4,096
bytes. Malformed stored values MUST retain ErrInvalidArchive classification.
The size guard and projection MUST use one evaluation of one matching value.
This bounds the Go string, not SQLite's internal schema-evaluation memory.

#### Scenario: An external application stores oversized metadata

- **WHEN** either archive metadata reader encounters more than 4,096 bytes
- **THEN** it returns ErrInvalidArchive, with any intermediate Go string bounded
  to at most 12,288 bytes

#### Scenario: A valid multibyte metadata value reaches the boundary

- **WHEN** the value is exactly 4,096 bytes without NUL
- **THEN** both metadata readers return the exact value

#### Scenario: The archive uses UTF-16 encoding

- **WHEN** a metadata value contains exactly 4,096 UTF-8 bytes in a UTF-16 archive
- **THEN** both metadata readers return the exact value
- **AND** values exceeding the UTF-8 byte limit return ErrInvalidArchive

#### Scenario: A legacy view changes its value on each evaluation

- **WHEN** a matching metadata view expression can return a different size
  each time it is evaluated
- **THEN** the reader evaluates that value once before guarding its projection
- **AND** a value over the encoded limit does not enter a Go string
