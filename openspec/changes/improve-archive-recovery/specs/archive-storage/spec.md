## ADDED Requirements

### Requirement: Observation-series event sensor counts

Each observation-series bucket MUST report `rain_readings` and
`lightning_readings` as the number of non-NULL readings for that sensor.
Existing numeric totals MUST remain sums of the available readings, with zero
for no readings. Callers MUST use the counts to interpret those totals.

#### Scenario: Missing and measured-zero samples

- **WHEN** a bucket contains NULL, zero, and positive event readings
- **THEN** each count includes zero and positive readings, excludes NULL, and its total
  includes only available values

#### Scenario: Independent sensor coverage

- **WHEN** only rain or only lightning is missing
- **THEN** the two counts describe each sensor independently without changing obs
