-- Claim an unbound archive for exactly one device. INSERT OR IGNORE makes the
-- first writer win atomically when two processes race to initialize a file;
-- the loser reads the winning value and refuses its mismatched device.
INSERT OR IGNORE INTO meta (key, value) VALUES ('archive_device_id', ?)
