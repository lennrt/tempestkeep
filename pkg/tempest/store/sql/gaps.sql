-- Coverage gaps: pairs of consecutive observations more than the threshold
-- apart, largest first. A cheap LAG window scan over the epoch index; an
-- archive with no qualifying gap returns no rows.
-- Params: minSeconds, limit.
SELECT prev, epoch, epoch - prev AS gap FROM (
    SELECT epoch, LAG(epoch) OVER (ORDER BY epoch) AS prev FROM obs_st
) WHERE prev IS NOT NULL AND epoch - prev > ?
ORDER BY gap DESC LIMIT ?
