-- Row count and epoch range of the whole archive.
SELECT COUNT(*), MIN(epoch), MAX(epoch) FROM obs_st
