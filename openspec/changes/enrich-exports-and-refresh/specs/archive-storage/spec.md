## ADDED Requirements

### Requirement: Read-only streaming observation metadata

The store MUST provide additive streaming access to observation metadata in
inclusive epoch order. It MUST preserve nullable stored values, classify invalid
arguments and archive errors, stop on cancellation or callback failure, and
close its rows. Missing optional columns in older schemas MUST yield missing
metadata. Source text MUST be valid UTF-8 and limited to 1,024 bytes before
loading it into Go memory.

#### Scenario: An older archive lacks metadata columns

- **WHEN** an observation is streamed with metadata
- **THEN** its available sensor values remain readable and unavailable metadata is missing

#### Scenario: A callback stops iteration

- **WHEN** the callback returns a sentinel error
- **THEN** iteration stops and the caller can identify the same error

#### Scenario: Stored source text exceeds the supported bound

- **WHEN** the metadata iterator reaches the row
- **THEN** it reports a classified error without loading or truncating the full source

#### Scenario: Stored source is not valid UTF-8

- **WHEN** the metadata iterator reaches an invalid text value
- **THEN** it reports a classified error rather than silently replacing bytes
