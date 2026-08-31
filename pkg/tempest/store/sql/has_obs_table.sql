-- Verify the archive actually has the collector's table (also acts as a
-- connectivity check).
SELECT name FROM sqlite_master WHERE type='table' AND name='obs_st'
