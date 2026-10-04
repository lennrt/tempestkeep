# Specification workflow

TempestKeep uses [OpenSpec][openspec] to record behavior and proposed changes.
A specification states a requirement and concrete examples of its expected result.
The Go tests provide evidence for those examples.

The baseline specifications live in [openspec/specs](../openspec/specs).
Active proposals live in [openspec/changes](../openspec/changes).
An OpenSpec proposal supplements the architecture decisions in [docs/adr](adr).

## Install the development tool

Use Node.js 20.19.0 or later with npm. The lockfile pins OpenSpec and its
dependencies. Node.js is needed only for specification work. Building and
running the Go application does not require it.

From the repository root, run these commands:

```sh
npm ci --ignore-scripts --no-audit --no-fund
npm run spec:list
npm run spec:check
```

`spec:check` validates all baseline specifications and active changes with
strict, non-interactive validation. The dedicated CI job runs the same command.
`make spec-check` is an alias after the npm dependencies are installed.

To disable OpenSpec telemetry for a shell session, set `OPENSPEC_TELEMETRY=0`.
In PowerShell, use `$env:OPENSPEC_TELEMETRY = '0'`. CI sets this automatically.

## Propose a behavior change

Before implementation, read the affected baseline specification and the relevant
ADRs. Create one change folder for one reviewable outcome. Use the CLI through
npm so that it uses the repository's pinned version:

```sh
npm run openspec -- new change improve-example-behavior
npm run openspec -- status --change improve-example-behavior
npm run openspec -- instructions proposal --change improve-example-behavior
```

The CLI creates the folder and shows instructions. It does not write the proposal
for you. Write the artifacts in that folder:

1. Write `proposal.md` with the problem, affected capabilities, and compatibility impact.
2. Write `specs/<capability>/spec.md` with requirement changes and WHEN/THEN scenarios.
3. Write `design.md` with the implementation decisions and failure behavior.
4. Write `tasks.md` with checkboxes that include each task's verification command or result.

Use `## ADDED Requirements` for new requirements. For an existing requirement,
use `## MODIFIED Requirements` and include its complete updated text and scenarios.
Use `#### Scenario:` headings so that OpenSpec can find the scenarios.

For a new capability, include a concrete `## Purpose` section before its added
requirements. Archive uses that text when it creates the baseline specification.
Without it, archive writes a placeholder that fails strict validation afterward.

Run `npm run spec:check` after editing the artifacts. Include tests with their
implementation tasks. Mark a task complete only after its stated check succeeds.
Record any environment limitation in the review evidence.

## Review and archive

Keep the change active while its pull request is under review. Link its folder
in the pull request. Use the scenarios to review failure cases, compatibility,
and the tests that protect each behavior.

After the change is accepted and all tasks are complete, archive it:

```sh
npm run openspec -- archive improve-example-behavior
npm run spec:check
```

Archive applies the requirement changes to `openspec/specs` and moves the
proposal into the dated archive directory. Review that diff before committing it.
Check each new capability's purpose and rerun strict validation after archiving.
Do not archive incomplete work to make the task list appear complete.

## Optional assistant integration

The committed workflow works with the CLI and any editor. To generate commands
for a specific assistant, run `npm run openspec -- init --tools <tool-id>`.
Use a supported tool ID from the [OpenSpec integration guide][tools].

Review generated assistant files before committing them. They are not needed to
build TempestKeep or to validate the committed specifications. Do not replace the
project context in `openspec/config.yaml` with a generic template.

## Plain-language review

Use [SimpleEnglish][simple-english] in Plain mode when editing instructions.
Use short sentences, active verbs, and one term for each meaning. Put a condition
before its action, and state prerequisites before a command.

Keep code, tool names, flags, paths, units, and quoted errors unchanged during a
prose-only pass. Check statements against implementation and executable examples.
This project does not claim formal ASD-STE100 compliance.

## What validation proves

OpenSpec validation checks the document structure and requirement scenarios.
It does not execute Go tests or prove that the implementation meets a requirement.
Run the checks in [CONTRIBUTING.md](../CONTRIBUTING.md) as well.

For a small spelling or link correction, update the document and run
`make docs-check`. A separate behavior proposal is not required when the
requirement and implementation remain unchanged.

[openspec]: https://github.com/Fission-AI/OpenSpec
[tools]: https://github.com/Fission-AI/OpenSpec/blob/main/docs/supported-tools.md
[simple-english]: https://github.com/AminBlg/SimpleEnglish
