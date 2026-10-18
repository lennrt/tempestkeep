## ADDED Requirements

### Requirement: Rain summary coverage across MCP and app views

rain_stats MUST expose a number-or-null total and observation/readings counts in
ordinary and structured results. Its output schema MUST allow null. Notes MUST
distinguish empty, missing, and partial coverage. The app MUST keep a rainfall
headline when its total is missing, preserve measured zero, and disclose partial
readings. Original results and unrelated tools MUST remain unchanged.

#### Scenario: A tool result has no rain measurements

- **WHEN** the host receives and displays rain_stats
- **THEN** JSON contains a null total and the app shows missing rainfall, not zero
- **AND** coverage counts remain visible without implying a dry period

#### Scenario: A refresh changes partial readings to measured zero

- **WHEN** the app receives the refreshed rain_stats result
- **THEN** it renders numeric zero with updated counts and removes the partial notice

#### Scenario: An older result has no coverage counts

- **WHEN** the app displays that result
- **THEN** existing numeric values retain their meaning without assumed zero counts
