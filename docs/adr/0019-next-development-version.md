# ADR 0019: Next development version

Status: proposed

## Context

The merged 0.4.0 development baseline precedes this batch of ten improvements.
The batch changes public rain nullability, collection selection, backup names,
and interactive forecast and table behavior. It adds an offline status command.

## Decision

Set the development version source and plugin manifest to 0.5.0. The app build
reads that same source. Untagged fallback builds report `v0.5.0-dev`. Preserve
linker and Go module version precedence. Preserve historical changelog, ADR,
and qualification evidence. Record the ten improvements and compatibility
effects in the new changelog section. Do not update dependencies for this bump.

## Consequences

Version 0.5.0 is a proposed development target. This decision does not authorize
a tag, release, plugin publication, deployment, or merge. Release qualification
and publication remain separate maintainer decisions under RELEASING.md.
