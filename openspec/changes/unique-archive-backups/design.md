# Design

Use an internal fixed-time snapshot helper so tests can reproduce same-clock
collisions without sleeps or shared clock state. Publish complete copies with
the existing hard-link operation. Retry only an existing-destination error,
using at most 100 candidate names. Clean temporary files on each failed attempt.
A collision never replaces the existing file. Other errors return immediately.

New names use UTC `20060102-150405.000000000`. Parse both this format and legacy
`20060102-150405` for retention. Compare parsed instants, then filenames for a
deterministic tie. Require exact format round-trips to exclude unrelated names.
Keep the existing directory-entry cap and regular-file/symlink protections.
No unbounded directory search or new dependency is needed. Clock rollback can
still make a new snapshot older than an existing one; retention follows the
recorded timestamps, as before.
