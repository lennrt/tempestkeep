## ADDED Requirements

### Requirement: Consistent 0.5.0 development metadata

The development version source, plugin manifest, embedded app identity, and
active user guidance MUST identify version 0.5.0. Builds without a module or
linker version MUST report `v0.5.0-dev`. Existing linker and module precedence
MUST remain effective. Historical release evidence MUST remain unchanged.
This change MUST NOT publish a release, tag, plugin, deployment, or merge.

#### Scenario: Untagged development build

- **WHEN** no module or linker version is supplied
- **THEN** the command reports `v0.5.0-dev`

#### Scenario: Shared app identity

- **WHEN** the app build reads the development version source
- **THEN** its embedded identity identifies version 0.5.0

#### Scenario: Historical evidence

- **WHEN** active development advances to 0.5.0
- **THEN** earlier release and qualification records retain their original versions
