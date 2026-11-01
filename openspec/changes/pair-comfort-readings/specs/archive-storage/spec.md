## ADDED Requirements

### Requirement: Paired comfort inputs

Comfort analytics MUST derive quarter-hour temperature/humidity means from
observations carrying both fields, and temperature/wind means from observations
carrying both fields. Dew point MUST use the humidity pair. Apparent temperature
MUST use the humidity pair at or above 80°F, the wind pair at or below 50°F,
and the mean of available temperatures strictly between those thresholds.
Every available candidate MUST contribute to daily apparent-temperature extrema.
Pairs MUST NOT cross quarter-hour buckets. Measured zero MUST remain distinct
from missing data.

#### Scenario: Required sensors occur in different observations

- **WHEN** a bucket contains hot temperature-only and humidity-only observations
- **THEN** it reports neither heat index nor dew point from those observations

#### Scenario: Unpaired readings accompany a complete pair

- **WHEN** extra temperature-only or humidity-only readings accompany one complete pair
- **THEN** the derived heat index and dew point use only the complete pair

#### Scenario: Cold air has no paired wind

- **WHEN** temperature and wind occur only in separate observations
- **THEN** they do not produce wind chill

#### Scenario: Mild air and measured zero

- **WHEN** mild temperature lacks other sensors or a required pair contains zero
- **THEN** mild temperature remains available and zero remains a measured input
