## ADDED Requirements

### Requirement: Timestamp-based latest observation

The client MUST validate every station observation and return the observation
with the greatest timestamp regardless of array ordering. The first response
row with that timestamp MUST win ties.

#### Scenario: Service sends newest observation first

- **WHEN** a station response is ordered newest to oldest
- **THEN** the client returns the newest timestamp

#### Scenario: Maximum timestamps tie

- **WHEN** multiple rows have the greatest timestamp
- **THEN** the first such row is returned

#### Scenario: Older row is malformed

- **WHEN** an older row fails validation
- **THEN** the whole response fails even if the newest row is valid
