## ADDED Requirements

### Requirement: Historical rain sample coverage
Historical daily, period and this-day summaries MUST expose obs and rain_readings and return
null rain totals when no rain readings exist. Rain normals MUST average only rain-contributing
months and expose rain_years. Counts MUST NOT imply complete temporal coverage.

#### Scenario: Missing and measured zero rain
- **WHEN** a historical bucket has observations but no rain readings
- **THEN** its rain total is null and rain_readings is zero
- **WHEN** it has measured zero rain
- **THEN** its rain total is numeric zero

#### Scenario: Partial rain and climate normals
- **WHEN** one year's month has rain readings and another has none
- **THEN** the normal uses only the rain-contributing year and reports rain_years as one
- **AND** partial sums remain available without claiming complete temporal coverage
