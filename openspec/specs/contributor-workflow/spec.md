# Contributor workflow

## Purpose

Keep implementation changes reviewable through deterministic checks, explicit
compatibility decisions, and instructions that contributors can reproduce.

## Requirements

### Requirement: Pure-Go production build

The application MUST build with CGO_ENABLED=0 without requiring a Node.js runtime.

#### Scenario: A contributor builds the command

- **WHEN** the contributor runs the documented Go build with CGO disabled
- **THEN** the command builds without installing JavaScript packages

### Requirement: Deterministic default tests

Default tests MUST use synthetic data and MUST NOT require a WeatherFlow token.

#### Scenario: Tests run without network credentials

- **WHEN** a contributor runs the standard test command without a token
- **THEN** API tests use local fixtures and return within the test timeout

### Requirement: Compatibility records

Changes to public APIs, wire fields, storage, security boundaries, or configuration
meaning MUST include an ADR and relevant regression tests.

#### Scenario: A change affects archive metadata semantics

- **WHEN** a contributor changes the meaning of a stored cursor
- **THEN** the review includes compatibility guidance and restart regressions
