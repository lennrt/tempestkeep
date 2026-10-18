## ADDED Requirements

### Requirement: Configurable default MCP device

The server MUST resolve a configured positive device ID to the exact accessible
ST device and its station. Resolution MUST be lazy, cache only success, preserve
cancellation, and reject unknown/non-ST choices without choosing another device.
Selection errors MUST precede collection writes and progress changes. Existing
archive mismatch checks and clearly labeled archive fallback MUST remain active.

#### Scenario: A token has two stations

- **WHEN** the operator selects the second station's Tempest device
- **THEN** default live station tools and collection use that selected station/device

#### Scenario: A station contains two Tempest devices

- **WHEN** the operator selects the second device
- **THEN** collection uses that exact device while live reads retain station-level semantics

#### Scenario: An explicit station read overrides the default

- **WHEN** a forecast or details request supplies another station ID
- **THEN** only that read uses the override and the configured default stays unchanged

#### Scenario: Selection cannot resolve

- **WHEN** the selected ID is unknown, non-Tempest, or unavailable
- **THEN** collection cannot change observations, binding, or progress and a later call can retry

#### Scenario: An archive opens offline

- **WHEN** a valid device selection exists without a live token
- **THEN** archive-only reads remain available

### Requirement: Searchable station device identities

The station app MUST provide a derived device table with parent station identity,
exact device identifiers, type, and serial when supplied. It MUST preserve raw
results and duplicates, cap traversal/display at 2,000 device entries, and report
omissions. Identifier displays and searches MUST retain ungrouped digits when
the received value is exact. Numeric IDs outside the browser's safe integer range
MUST display a precision warning instead of a rounded identifier. Text IDs MUST
remain unchanged. The view MUST use existing local search/sort/page and refresh
rules without tool calls.

#### Scenario: A user searches for a device ID

- **WHEN** the user pastes the ungrouped ID in the Devices table search
- **THEN** the matching device and parent station are visible

#### Scenario: A numeric identifier exceeds browser precision

- **WHEN** an ID is a numeric integer above the browser's safe integer range
- **THEN** its cell and tooltip report `ID exceeds browser precision`
- **AND** exact text IDs and raw results remain unchanged

#### Scenario: A station contains hostile or missing fields

- **WHEN** the table renders those device entries
- **THEN** values remain text, missing fields stay missing, and no extra request is made

#### Scenario: Device entries exceed the display limit

- **WHEN** the app derives the table
- **THEN** work and rows stay bounded and the view identifies omitted entries
