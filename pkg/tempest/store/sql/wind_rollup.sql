-- Per-bucket wind rollup: one scan groups observations into fixed 15-minute
-- epoch buckets (see rollupBucketSeconds), and Go merges the buckets into local
-- calendar days for the daily means and extremes. The bucket mean and its count
-- let Go form an observation-weighted daily average; the calm count (average
-- wind below the calm threshold) rolls up to a range-wide calm share.
-- Params: rollupBucketSeconds, calmThresholdMps, startEpoch, endEpoch.
SELECT epoch/? AS b,
       COALESCE(SUM(wind_avg), 0), COUNT(wind_avg),
       MAX(wind_avg), MAX(wind_gust),
       SUM(CASE WHEN wind_avg < ? THEN 1 ELSE 0 END)
FROM obs_st
WHERE epoch BETWEEN ? AND ?
GROUP BY b
ORDER BY b
