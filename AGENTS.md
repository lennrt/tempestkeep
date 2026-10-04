# Working on TempestKeep

Read [CONTRIBUTING.md](CONTRIBUTING.md) before changing the project.
Read the relevant specifications in [openspec/specs](openspec/specs).
Read active proposals in [openspec/changes](openspec/changes).

For a behavior change, create or update an OpenSpec proposal, scenarios,
design, and tasks. Use [the workflow guide](docs/openspec.md).
An OpenSpec proposal supplements the ADR rules in CONTRIBUTING.md.
For a small text correction, update the document and run `make docs-check`.

Keep each regression test with the fix that it protects.
Use synthetic fixtures. Keep tokens and real station data out of the repository.
Record the exact checks that ran and the checks that remain unavailable.
