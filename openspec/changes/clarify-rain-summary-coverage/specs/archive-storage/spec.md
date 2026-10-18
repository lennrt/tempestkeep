## ADDED Requirements

### Requirement: Rain summary measurement coverage

RainStats MUST count stored observations and nonmissing rain readings in the
inclusive requested range. Its total MUST be null when no rain readings exist,
and MUST otherwise sum reported rainfall, including a measured zero. It MUST NOT
infer values for missing samples. Counts MUST describe samples, not elapsed uptime.
Existing day, threshold, wettest-day, and spell semantics MUST remain unchanged.

#### Scenario: All stored rain readings are missing

- **WHEN** observations exist but none contains rain
- **THEN** the total is null, observations is positive, and rain_readings is zero

#### Scenario: Rain is measured as zero

- **WHEN** at least one rain reading is zero and none reports positive rain
- **THEN** the total is numeric zero and rain_readings counts those measurements

#### Scenario: Only some observations report rain

- **WHEN** the range mixes measured rain and missing values
- **THEN** the total sums only reported readings and the counts expose partial coverage

#### Scenario: The selected range is empty

- **WHEN** no observation belongs to its inclusive bounds
- **THEN** both counts are zero and the total is null

#### Scenario: Rain readings lie outside the requested range

- **WHEN** those readings surround an all-missing selected range
- **THEN** they affect neither its total nor its coverage counts
