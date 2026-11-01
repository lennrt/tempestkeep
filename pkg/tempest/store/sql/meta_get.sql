-- Read one value from the archive's key/value meta table.
-- Bound hostile external values before the driver allocates a Go string.
-- SQLite BLOB casts use the database encoding, which can be UTF-16. Allow
-- twice the UTF-8 byte limit here; readMetadata enforces the UTF-8 limit.
-- Materialize one value before measuring it so legacy views cannot reevaluate
-- a nondeterministic expression between the size guard and the projection.
-- Params: key, 2 * MaxMetadataValueBytes.
WITH metadata_value AS MATERIALIZED (
    SELECT value FROM meta WHERE key=? LIMIT 1
)
SELECT CASE WHEN length(CAST(value AS BLOB)) <= ? THEN value ELSE NULL END,
       length(CAST(value AS BLOB))
FROM metadata_value
