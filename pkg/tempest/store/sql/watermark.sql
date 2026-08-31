-- Newest stored epoch for one device (NULL when it has no rows). An
-- incremental sync fetches only observations after this.
SELECT MAX(epoch) FROM obs_st WHERE device_id=?
