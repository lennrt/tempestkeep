-- Upsert one key/value pair into the meta table.
INSERT INTO meta (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value
