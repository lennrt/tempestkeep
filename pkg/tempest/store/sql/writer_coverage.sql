-- Row count and epoch span stored for one device.
SELECT COUNT(*), MIN(epoch), MAX(epoch) FROM obs_st WHERE device_id=?
