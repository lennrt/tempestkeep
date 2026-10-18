## ADDED Requirements

### Requirement: Nearest eligible pressure comparison

Pressure tendency MUST compare the latest nonmissing pressure with the nearest
eligible earlier pressure to the requested window start. Eligible pairs MUST be
separated by at least half the requested window and have a positive span. Ties
MUST choose the older sample. The result MUST retain actual elapsed span and
normalize change to three hours. No eligible pair MUST produce no trend.

#### Scenario: The nearest sample follows the window start

- **WHEN** it is closer than the predecessor and satisfies the minimum span
- **THEN** the trend uses that sample rather than an older opposite-direction reading

#### Scenario: History covers more than half but less than the whole window

- **WHEN** an eligible pressure pair exists
- **THEN** a trend is returned with its actual span and normalized rate

#### Scenario: Candidate samples tie

- **WHEN** equally distant eligible samples surround the target
- **THEN** the older sample is selected deterministically

#### Scenario: A one-second window has only one sample

- **WHEN** pressure tendency is requested
- **THEN** it returns no trend instead of dividing by a zero span
