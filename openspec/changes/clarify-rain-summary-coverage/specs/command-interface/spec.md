## ADDED Requirements

### Requirement: Rain coverage in archive reports

Text reports MUST distinguish unavailable rainfall, measured zero, and partial
recorded totals. JSON MUST include total_in as number-or-null and both sample
counts. Missing rain MUST NOT hide other available weather sections. Reports
MUST NOT claim full temporal coverage from sample counts.

#### Scenario: A range contains observations without rain

- **WHEN** stats renders text and JSON
- **THEN** text states that rain is unavailable and JSON contains total_in null
- **AND** other weather sections remain available

#### Scenario: A partially reported total is zero

- **WHEN** stats renders the range
- **THEN** zero remains a measured total with explicit partial-reading information
