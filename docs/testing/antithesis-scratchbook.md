# Antithesis workload scratchbook

No Antithesis run has been submitted. This document records workload design
only. The source baseline is
`5ee17ef57463d366171b0e83dbbf7cd22775cd52`. The design also covers the changes
described by this review.

## System model

The smallest useful topology has three components:

1. A deterministic HTTP service that implements the WeatherFlow endpoints.
2. One TempestKeep process with a writable SQLite archive on a persistent
   volume.
3. A workload process that issues collection and read operations and checks the
   archive through public interfaces.

Add a second writer only for the one-device ownership race. Keep test-only fault
controls outside production code.

## Workload state

Generate observations from a deterministic seed. The model records:

- the generated `(device_id, epoch)` keys;
- the first value assigned to each key;
- the latest fully acknowledged chunk;
- the active device claim;
- the persisted cursor and completion marker; and
- the expected count and minimum and maximum epochs.

Bound one run to 10,000 generated observations, 100 collection operations, two
writers, and one archive. Log the seed before the first operation. Do not log
station data or a real token.

## Operations

- Insert a new observation chunk.
- Replay a completed chunk.
- Resume from a prior cursor.
- Cancel during fetch, throttle, transaction, checkpoint, or progress reporting.
- Restart the TempestKeep process after an acknowledged or ambiguous call.
- Return HTTP 429, HTTP 5xx, malformed JSON, oversized JSON, delayed data, a
  dropped connection, or a partial body.
- Race two device claims.
- Query coverage and sample stored rows.
- Close and reopen read and write handles.

## Properties

Use the IDs in [properties.md](properties.md). Continuous assertions should
cover P-01 through P-06 and P-13. End-of-run assertions should cover P-08,
P-10, P-11, P-14, and P-17. An ambiguous client outcome can be replayed; the
model must not assume that the first attempt failed.

## Fault plan

Start with process termination and HTTP outages. Add malformed responses and
connection truncation. Add disk-full, read-only-volume, and delayed-filesystem
faults only when the Antithesis environment exposes those controls. Do not
simulate a fault by weakening production checks.

## Required inputs before implementation

- The Antithesis project name and access path.
- An approved Docker Compose topology.
- Confirmation that the target environment provides persistent-volume and
  network fault controls.
- `snouty` output for the proposed configuration.
- Explicit approval to submit a run.

Until those inputs exist, native tests are the executable evidence. Do not claim
an Antithesis result.
