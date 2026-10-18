## ADDED Requirements

### Requirement: Preserve compatible table exploration on refresh

The app MUST preserve the selected dataset, search, sort, direction, and valid
page when a successful result refresh retains that dataset's ordered column
identities. It MUST use the controls present at result arrival, clamp pages to
the refreshed filtered rows, and restore focused compatible table controls.
New tool input or incompatible table structure MUST reset table controls.
Failed refreshes MUST preserve the previous result and controls. State MUST
remain local to the current app instance and received rows.

#### Scenario: Filtered and sorted rows refresh

- **WHEN** a compatible table refresh returns new rows
- **THEN** search and sort still apply and the page is clamped to available matches

#### Scenario: A user types while refresh is pending

- **WHEN** the refreshed result arrives while the search field has focus
- **THEN** the current text and selection survive with focus on that field

#### Scenario: The selected table disappears or columns change

- **WHEN** a refreshed result no longer has the compatible selected dataset
- **THEN** table controls reset safely instead of using stale column positions

#### Scenario: A new tool input begins

- **WHEN** the host supplies a new request context
- **THEN** previous table filters and sorting do not affect its result

#### Scenario: A refresh fails

- **WHEN** the host reports a tool error or rejects the call
- **THEN** the previous result, table controls, and failure notice remain available
