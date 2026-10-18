## ADDED Requirements

### Requirement: Consistent 0.4.0 development metadata

The development version source, plugin manifest, embedded app identity, and
active user guidance MUST identify version 0.4.0. Builds without a module or
linker version MUST report `v0.4.0-dev`. Existing linker and module version
precedence MUST remain effective. This change
MUST NOT create a tag, release, deployment, plugin publication, or merge.

#### Scenario: A contributor builds the untagged source

- **WHEN** no module or linker version is supplied
- **THEN** the command reports `v0.4.0-dev`

#### Scenario: An app bundle is generated

- **WHEN** the build reads the development version source
- **THEN** the embedded app identity matches version 0.4.0
