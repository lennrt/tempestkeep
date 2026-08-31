-- Per-bucket pressure rollup: one scan groups observations into fixed 15-minute
-- epoch buckets (see rollupBucketSeconds), and Go merges the buckets into local
-- calendar days for the daily mean (from the bucket sum and count) and the daily
-- pressure extremes. The day-over-day change in the daily mean is the storm-swing
-- signal PressureStatistics reports.
-- Params: rollupBucketSeconds, startEpoch, endEpoch.
SELECT epoch/? AS b,
       COALESCE(SUM(pressure_mb), 0), COUNT(pressure_mb),
       MIN(pressure_mb), MAX(pressure_mb)
FROM obs_st
WHERE epoch BETWEEN ? AND ?
GROUP BY b
ORDER BY b
