## ADDED Requirements

### Requirement: Opt-in observation export metadata

The export command MUST accept `--metadata` to append stored `device_id`,
`report_interval_min`, `wind_interval_s`, `precip_type`, and `source` to each
CSV or JSON Lines observation. It MUST preserve default export output, numeric
precision, missing values, and metadata units under SI and US selection.

#### Scenario: A user reproduces a solar-energy estimate

- **WHEN** metadata export contains 100 W/m² radiation and a one-minute interval
- **THEN** the exported values yield 0.006 MJ/m² without guessing a sample duration

#### Scenario: Stored metadata is missing or zero

- **WHEN** a metadata field is NULL or a stored numeric zero
- **THEN** NULL is an empty CSV cell or omitted JSON field and zero stays zero

#### Scenario: The user omits the metadata flag

- **WHEN** the user exports a supported existing archive
- **THEN** the CSV header, column order, and JSON field shape remain unchanged

#### Scenario: Source text needs escaping

- **WHEN** a stored source contains quotes, newlines, or control characters
- **THEN** each export remains valid in its selected format and preserves the text
