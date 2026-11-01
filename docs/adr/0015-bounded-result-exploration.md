# ADR 0015: Bounded result exploration

Status: proposed

## Context

SQL cell previews truncate strings before search and do not expose the hidden
suffix. Dataset selection replaces the focused button. Daily forecasts offer
charts and tables without daily summary cards.

## Decision

Keep compact previews and add an accessible native modal for long SQL strings.
Search and expanded inspection share a 65,536-character per-cell limit. Disclose
that limit when received values exceed it. Keep existing row and column limits,
render untrusted strings as text, and preserve raw tool results. Restore focus
on dataset changes and after modal dismissal. Show daily forecast high, low,
maximum rain probability, and returned day count using available values.

## Consequences

SQL search can find more received text while remaining bounded. Oversized values
still require inspection through the original tool result. UI changes add no
network requests, runtime dependencies, tool schemas, or archive writes.
