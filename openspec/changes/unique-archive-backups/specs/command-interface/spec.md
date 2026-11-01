## Purpose

Create private archive snapshots after collection without replacing prior backups.

## ADDED Requirements

### Requirement: Collision-safe archive snapshots

Collection backups MUST publish complete files without replacing existing files.
New names MUST include UTC nanosecond precision. On a name collision, backup
creation MUST try at most 100 candidate names before returning an error. Failed
attempts MUST remove their temporary files. Retention MUST recognize both legacy
second-precision names and new names and order them by parsed timestamp.

#### Scenario: Repeated backups use the same clock instant

- **WHEN** multiple snapshots use the same clock instant
- **THEN** each successful snapshot has a distinct name and preserves prior bytes

#### Scenario: Legacy and new snapshots share a retention directory

- **WHEN** retention removes older snapshots
- **THEN** it orders both formats by timestamp and preserves unrelated files

#### Scenario: All candidate names already exist

- **WHEN** all 100 candidate names collide
- **THEN** backup creation fails without replacing files or leaving temporary files
