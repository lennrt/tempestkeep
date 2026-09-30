## ADDED Requirements

### Requirement: Repeatable specification checks

The repository MUST pin the OpenSpec development dependency and provide a strict,
non-interactive validation command for baseline specifications and active changes.

#### Scenario: A contributor installs the locked development dependencies

- **WHEN** the contributor runs npm ci followed by npm run spec:check
- **THEN** the pinned CLI checks every baseline specification and active change

### Requirement: Accurate fenced documentation examples

The documentation checker MUST distinguish outer Markdown fences from shorter
fences inside an example. It MUST require a matching closing delimiter.

#### Scenario: A Markdown example contains a nested code fence

- **WHEN** a four-backtick fence contains a three-backtick example
- **THEN** the checker treats the inner fence and its links as example content

### Requirement: Correct local link decoding

The documentation checker MUST decode local URL paths once and preserve literal
encoded characters in filenames. External web links MUST use explicit HTTPS URLs.

#### Scenario: A filename contains a literal percent sequence

- **WHEN** a link encodes the percent sign in that filename
- **THEN** the checker resolves the intended file without a second decoding pass

### Requirement: Execute documented query examples

The test suite MUST read worked query examples from the documentation and execute
them against synthetic data through the MCP transport. Tests MUST check boundary
dates, missing readings, and unit conversions.

#### Scenario: A rain example selects one calendar month in UTC

- **WHEN** the test archive includes large rain readings just outside that month
- **THEN** the example excludes them and returns the expected coverage and total
