## ADDED Requirements

### Requirement: Historical rain coverage display

History tool views MUST preserve null rainfall and measured zero, retain available
partial totals, and explain missing or partial sample counts. Derived rainy-day
display values MUST be unavailable when no rain readings exist. Display copies
MUST preserve the original tool result and arbitrary SQL values.

#### Scenario: Historical rows contain missing, zero, and partial rain

- **WHEN** the app shows daily, period, or this-day rows with coverage counts
- **THEN** missing rain displays as unavailable and partial totals retain their values
- **AND** a notice explains that sample counts do not establish continuous coverage

#### Scenario: Climate normals contain rain-contributing years

- **WHEN** the app displays rain_years
- **THEN** it explains that partial months can contribute to the reported mean
- **AND** a missing rain normal remains unavailable

#### Scenario: Refresh replaces incomplete history with reported zero

- **WHEN** the same history request returns zero rain with every sample reported
- **THEN** the old incomplete-coverage notice disappears and measured zero remains visible
