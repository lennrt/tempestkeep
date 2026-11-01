## ADDED Requirements

### Requirement: Accessible bounded result exploration

The app MUST preserve focus on the selected dataset control after a user switches
datasets. SQL string previews MUST support an accessible expanded inspection and
search up to 65,536 characters per cell. Longer values MUST disclose the shared
inspection and search limit without mutating the original result. The inspector
MUST close by keyboard and restore focus to its originating control when present.

#### Scenario: Keyboard dataset change

- **WHEN** a user activates another dataset with the keyboard
- **THEN** its replacement dataset button retains focus and selected state

#### Scenario: Long SQL string

- **WHEN** a returned SQL string contains a search term after its compact preview
- **THEN** search finds that term within the inspection bound and the user can inspect it as text

#### Scenario: Oversized SQL string

- **WHEN** a SQL string exceeds 65,536 characters
- **THEN** the table and inspector disclose the bound and neither searches nor displays the
  suffix beyond it

### Requirement: Daily forecast summaries

The app MUST summarize daily forecast highs, lows, maximum rain probability, and
returned day count when the daily dataset is selected. Missing measurements MUST
remain distinct from zero and hourly summaries MUST retain hourly wording.

#### Scenario: Daily forecast selection

- **WHEN** the user selects daily forecast rows containing missing and zero readings
- **THEN** summaries use available daily values without invented measurements or additional tool
  calls
