## Why

Repeated collection can fail after station list ordering changes. Explicit zero
selectors also silently invoke fallback. Latest station readings trust array
order rather than timestamps.

## What Changes

- Reuse existing archive identity when collection has no device selector.
- Read identity through the shared metadata reader, including UTF-16 databases
  and case-insensitive legacy table names.
- Reject explicit nonpositive selectors and nonempty nonpositive environment IDs.
- Select the greatest validated station timestamp; first row wins ties.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `command-interface`: preserve collection identity and validate selectors.
- `weather-api`: choose newest validated observation regardless of response order.

## Impact

Only default collection selection and latest response ordering change. Empty or
missing archives retain discovery. Explicit positive selection retains writer
mismatch errors. Zero must be omitted to request fallback. See ADR 0018.
