## Decisions

Inspect existing archives read-only before discovery. Read persisted identity,
otherwise use a bounded first device query after Store validates one-device
ownership. Use the shared bounded metadata reader so database encoding and
case-insensitive metadata table names match writer binding. Validate metadata
as a positive integer. Missing paths discover;
other file/read errors do not fall through. Empty modern archives discover.
Unidentifiable legacy schemas require an explicit selector. Close the reader
before Writer opens. Writer binding still rejects mismatch and concurrent claims.

Validate all API rows before scanning for the largest timestamp. Use a strict
greater-than comparison so the first matching maximum wins ties, without sorting.

## Security and compatibility

No logs include IDs, paths, tokens, or response bodies. Selection reads do not
create an archive. Explicit positive selectors retain precedence. Nonempty zero
environment values now fail, consistent with the positive-ID contract.
