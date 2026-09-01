---
description: Build or extend the local weather archive in bounded batches
argument-hint: "[oldest date to reach, YYYY-MM-DD]"
---

Build the local weather archive with the `tempestkeep` MCP archive tools. Each
`backfill_archive` call fetches one bounded batch and is safe to repeat.

Target: $ARGUMENTS (if empty, walk back until the station's history runs out).

1. `archive_status` first: report current coverage (span, row count, gaps) so
   progress has a baseline. If the status shows the backfill already reached the
   target, say so and stop.
2. Call `backfill_archive` at most 12 times in this session:
   - Pass `start` only if the user gave a target date.
   - Leave `end` unset; the tool resumes from the oldest data already stored.
   - Keep the default `max_days`.
   - After each call, report the covered dates, added rows, and running total.
     Do not print raw JSON.
3. Stop when the tool reports completion, reaches the target, returns an error,
   or reaches the 12-call limit. If work remains, report the resume state. If it
   completed, finish with:
   - `sync_archive`: top up to the present.
   - `archive_status`: final coverage; mention any gaps worth a targeted re-run.
   - One flourish from `records` (e.g. "your all-time gust: 43 mph, Jan 2024").

If a call fails, report the failure and the saved resume point. Do not retry in a
tight loop. If the server is read-only or has no token, point the user at
`/tempestkeep:setup`.
