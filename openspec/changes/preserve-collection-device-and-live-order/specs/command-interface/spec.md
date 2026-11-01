## ADDED Requirements

### Requirement: Stable collection device selection

Collection MUST prefer a positive explicit device flag, then a positive nonempty
environment selector, then an existing archive identity, then live discovery.
Explicit nonpositive flags MUST fail before configuration or external work.
Invalid existing archive identity MUST fail without discovery. Missing or empty
unbound archives MUST retain discovery. Legacy archives without identifiable
device columns MUST require explicit selection. Writer ownership checks MUST
continue to reject explicit mismatches.

#### Scenario: Stations reorder between scheduled collections

- **WHEN** an existing bound archive is collected without selectors
- **THEN** collection reuses its device without requesting the station list

#### Scenario: Explicit zero and broken dotenv

- **WHEN** collection receives --device-id 0 and invalid dotenv configuration
- **THEN** a usage error is returned before reading configuration or creating files

#### Scenario: Fresh archive has no selector

- **WHEN** collection targets a nonexistent archive
- **THEN** live discovery selects its device

#### Scenario: Explicit selection disagrees with archive

- **WHEN** a positive selector identifies another device
- **THEN** collection returns a device mismatch without changing observations
