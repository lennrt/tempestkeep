## ADDED Requirements

### Requirement: Seasonal trend with uneven month coverage

TemperatureTrend MUST fit a common yearly slope with separate calendar-month
intercepts and equal weight per usable monthly mean. It MUST center time and
temperature by calendar month. A month seen once MUST NOT change the slope.

#### Scenario: A later unmatched month is added

- **WHEN** two January means one year apart differ by 3.6°F and a later December
  is observed only once
- **THEN** the slope remains 36°F per decade with within-month R² of one

#### Scenario: Repeated temperatures are constant

- **WHEN** a calendar month repeats across years with no temperature variation
- **THEN** the slope is zero and R² is absent

#### Scenario: No month repeats

- **WHEN** usable months never share a calendar month across years
- **THEN** the slope and R² are absent because no within-month trend is identifiable
