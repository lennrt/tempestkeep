-- Read one value from the archive's key/value meta table.
-- Bound hostile external values before the driver allocates a Go string.
-- Params: MaxMetadataValueBytes, key.
SELECT CASE WHEN length(CAST(value AS BLOB)) <= ? THEN value ELSE NULL END,
       length(CAST(value AS BLOB))
FROM meta WHERE key=?
