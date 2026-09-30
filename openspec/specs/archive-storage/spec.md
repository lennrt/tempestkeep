# Archive storage

## Purpose

Keep one station device's observations in a local, reusable SQLite archive.
Protect sensor meaning and support repeated collection without duplicate rows.

## Requirements

### Requirement: One device per archive

The archive MUST reject writes for a second device once it contains observations.

#### Scenario: A different device is collected

- **WHEN** a writer receives a device ID that differs from the archive's device
- **THEN** it rejects the write and preserves the existing observations

### Requirement: SI storage and missing values

The archive MUST store sensor values in SI units and preserve missing values as NULL.

#### Scenario: An observation lacks a temperature

- **WHEN** the API reports a missing temperature
- **THEN** the archive stores NULL rather than a measured zero

### Requirement: Replay-safe observation writes

Observation writes MUST use the device ID and epoch as their unique key.
Collection MUST commit a chunk before recording progress past it.

#### Scenario: Collection restarts after interruption

- **WHEN** collection requests a previously committed chunk again
- **THEN** the archive contains no duplicate observation keys

### Requirement: Read-only analytical access

Store analytical operations MUST use a read-only database handle.

#### Scenario: A caller submits a write through analytical SQL

- **WHEN** a query attempts to modify the archive
- **THEN** the operation fails and the archive remains unchanged
