## ADDED Requirements

### Requirement: Exact explicit backfill continuation

The backfill_archive tool MUST accept an optional positive exclusive
`before_epoch` within the supported observation range. It MUST reject combining
it with `end` and reject a start at or beyond the exclusive boundary before
external work. Explicit bounds MUST NOT change shared backfill state.

#### Scenario: Continue a partial daylight-saving day

- **WHEN** a bounded call returns a non-midnight next_before cursor
- **THEN** passing that cursor as before_epoch with the same start continues exactly
  at that boundary without skipped observations

#### Scenario: Explicit continuation leaves shared progress alone

- **WHEN** a client calls with end or before_epoch
- **THEN** the shared cursor, completion marker, and empty-history progress remain unchanged

### Requirement: Observation event coverage display

Observation-series results MUST include independent rain and lightning reading
counts. The app MUST treat an event total with zero readings as unknown and
preserve a measured zero with positive readings. It MUST indicate partial
coverage when a sensor count is below the bucket observation count.

#### Scenario: Missing rain differs from measured dry weather

- **WHEN** a series contains both zero-reading and measured-zero rain buckets
- **THEN** the app omits the unknown chart value and displays the measured zero

#### Scenario: Some sensor samples are missing

- **WHEN** a bucket has fewer event readings than observations
- **THEN** the app displays available totals with an incomplete-coverage notice

#### Scenario: Older result has no coverage fields

- **WHEN** the host sends a result without the new reading counts
- **THEN** the app preserves its previous numeric display behavior
