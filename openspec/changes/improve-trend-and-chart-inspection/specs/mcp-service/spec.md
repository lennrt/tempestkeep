## ADDED Requirements

### Requirement: Accessible chart reading inspection

The app MUST let users inspect every returned series point using pointer or
touch and keyboard controls, up to the existing 2,000-point display limit.
The inspector MUST show the selected source timestamp, metric, units, and
missing-value state without interpolating values or calling another tool.

#### Scenario: Dense observation series

- **WHEN** a chart has more than 60 points
- **THEN** users can select its first, intermediate, and last readings directly

#### Scenario: Missing and zero readings

- **WHEN** keyboard selection moves between a missing reading and a measured zero
- **THEN** the inspector distinguishes them and preserves chart gaps

#### Scenario: Result or metric changes

- **WHEN** a new result or metric replaces the chart
- **THEN** selection resets or clamps to valid rows and shows no stale reading

#### Scenario: Narrow touch viewport

- **WHEN** the app is displayed at 390 pixels wide
- **THEN** the inspector remains readable and touch selection does not widen the document
