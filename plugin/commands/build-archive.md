---
description: Build or extend the local weather archive in bounded batches
argument-hint: "[oldest date to reach, YYYY-MM-DD]"
---

Build the local archive with the `tempestkeep` MCP tools. Use at most 12 archive
write calls in this session, counting both backfill and sync. Honor any smaller
budget that the user requests.

Target: $ARGUMENTS. If empty, walk backward until the API's available history
ends. Do not claim that the API contains every observation from the station's
lifetime.

1. Discover the tools that are available. If write tools are absent, explain
   which configuration is missing and point to `/tempestkeep:setup`.
2. Call `archive_status`. Report the stored date span, row count, and large gaps.
3. If older history is needed, call `backfill_archive`. Pass `start` only when
   the user supplies a target date. Leave `end` and `before_epoch` unset to use
   the saved cursor.
   Keep the default `max_days`.
4. After each successful call, report covered dates and rows added. If
   `has_more` is true and the call budget remains, repeat the same arguments.
   Stop backfill on completion, an error, or the call limit.
5. If backfill succeeds and the call budget remains, call `sync_archive`.
   Repeat while `has_more` is true and the call budget remains.
6. Call `archive_status` again. Report coverage, freshness, remaining gaps,
   and whether the session stopped before collection finished.

If a call fails, report the error without sensitive identifiers. Committed rows
remain available. Do not claim that a tool error includes a usable resume
result. A backfill call without `end` or `before_epoch` resumes saved progress.
Keep the same `start` when the user supplied a target date. Do not retry
failures in a tight loop.

If the oldest stored date already reaches the target, skip older backfill and
continue with sync. That date alone does not prove complete coverage. Report
large gaps separately. An explicit `end` or `before_epoch` gap repair does not
advance the shared resume cursor. If it returns `has_more`, pass `next_before`
as `before_epoch` and repeat `start` and `max_days`. Omit `end` on that call.
Do not convert the cursor to a date because it can fall within a local day.
After an error, repeat the failed explicit request. Committed rows remain safe
to replay.

End with the number of write calls used and any remaining work. If records
help answer the user's question, label them as records within this archive.
Do not imply that this command installs a background collector.
