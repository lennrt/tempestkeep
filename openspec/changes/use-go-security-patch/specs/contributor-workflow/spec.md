## MODIFIED Requirements

### Requirement: Consistent Go patch toolchain

The module, repository verification, CI, release qualification, and active build
instructions MUST use Go 1.27.3. Repository verification MUST reject a different
Go patch version. Historical evidence MUST NOT attribute older test runs to the
newly pinned toolchain.

#### Scenario: A contributor verifies with the required patch

- **WHEN** the selected Go toolchain reports go1.27.3
- **THEN** the version guard permits repository verification to continue

#### Scenario: A contributor verifies with a different patch

- **WHEN** the selected Go toolchain reports a version other than go1.27.3
- **THEN** the version guard fails before running the verification suite

#### Scenario: A maintainer updates the active toolchain pin

- **WHEN** a dated test record predates the patch upgrade
- **THEN** the record identifies its results as historical rather than results for the new patch
