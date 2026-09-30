## ADDED Requirements

### Requirement: Complete checkpoints before backup

Writer.Checkpoint MUST report an archive I/O error when SQLite reports a busy
reader or incomplete checkpoint. The CLI MUST NOT create a backup after that failure.

#### Scenario: A reader pins a WAL snapshot

- **WHEN** SQLite cannot checkpoint all frames because another reader is active
- **THEN** Checkpoint fails without claiming that the database file is safe to copy

#### Scenario: The reader releases its snapshot

- **WHEN** a later checkpoint completes
- **THEN** it succeeds and the committed observations remain readable

### Requirement: Aligned series output limits

Series queries MUST count the actual epoch-aligned buckets intersected by a range
before comparing that count with MaxSeriesPoints.

#### Scenario: A short range crosses an extra bucket boundary

- **WHEN** a range spans more than MaxSeriesPoints aligned buckets
- **THEN** the store rejects the range before allocating its result

### Requirement: Bound arbitrary SQL results inside SQLite

The store MUST apply per-query SQLite value and column limits before evaluating
arbitrary analytical SQL and restore prior connection limits before reuse.

#### Scenario: A query constructs a value above the byte limit

- **WHEN** SQL requests a zeroblob larger than the result limit
- **THEN** the query returns ErrResultTooLarge without constructing that large result

#### Scenario: A query is canceled

- **WHEN** the query context reaches its deadline
- **THEN** the connection's previous limits are restored before another caller uses it

### Requirement: JSON-safe SQL numbers

Arbitrary SQL results MUST reject non-finite floating-point values with a sanitized error.

#### Scenario: A query evaluates an infinite number

- **WHEN** SQLite returns infinity for a numeric expression
- **THEN** the store returns an error instead of a result that JSON cannot encode

### Requirement: Fractional wind sectors and separate coverage denominators

Wind sectors MUST retain fractional direction values. Calm percentage MUST use
all rows with wind speed. Sector percentages MUST use non-calm rows with direction.

#### Scenario: A direction is at the fractional NNE boundary

- **WHEN** a non-calm sample has direction 11.25 degrees
- **THEN** it belongs to NNE rather than N

#### Scenario: A non-calm sample lacks a direction

- **WHEN** wind statistics include that sample
- **THEN** it contributes to the calm denominator but not directional sector percentages

### Requirement: Solar energy estimates use reported intervals

Solar energy estimates MUST sum irradiance times positive report intervals.
Missing or zero intervals MUST contribute no energy. Sensor peaks MUST remain
available. Query inclusion and local-day attribution MUST use sample timestamps.

#### Scenario: A sparse sample reports a one-minute interval

- **WHEN** a 100 W/m² sample is the only reading in its 15-minute bucket
- **THEN** its estimated energy contribution is 0.006 MJ/m² rather than 0.09 MJ/m²

#### Scenario: An irradiance sample has no report interval

- **WHEN** solar statistics aggregate the sample
- **THEN** its irradiance can set a peak but cannot add an assumed energy duration

#### Scenario: A report interval crosses a query boundary

- **WHEN** its sample timestamp is included in the query
- **THEN** the estimate includes its whole reported interval without claiming boundary clipping

### Requirement: Resumable empty-history progress

Backfill progress MUST survive bounded calls and process restarts without marking
a single small empty request as the start of history.

#### Scenario: A caller repeatedly uses a small work budget

- **WHEN** successful empty requests continue across several backfill calls
- **THEN** their persisted progress eventually reaches the same exhaustion rule

#### Scenario: A chunk fails before commit

- **WHEN** collection fails before saving a chunk
- **THEN** the persisted cursor does not advance past that chunk

### Requirement: Missing sensors do not imply measured weather

Dry and storm-free spells MUST break on days with no readings from their relevant
event sensor. Apparent temperatures MUST use the sensors required for their
temperature band. Existing public day counts and result shapes MUST remain stable.

#### Scenario: A day has observations but no rain readings

- **WHEN** dry days appear before and after that day
- **THEN** the missing day breaks the dry spell instead of extending it

#### Scenario: A day has observations but no lightning count readings

- **WHEN** storm-free days appear before and after that day
- **THEN** the missing day breaks the storm-free spell

#### Scenario: A hot observation has no humidity reading

- **WHEN** comfort statistics calculate apparent-temperature extremes
- **THEN** the observation contributes no invented heat-index value
