-- Per-bucket lightning rollup: one scan groups observations into fixed 15-minute
-- epoch buckets (see rollupBucketSeconds), and Go merges the buckets into local
-- calendar days. strike_count is the strikes detected in a reporting interval;
-- strike_dist_km is the distance to those strikes, meaningful only when strikes
-- occurred, so the distance aggregates ignore zero-strike (and non-positive
-- distance) rows rather than dragging the minimum to zero.
-- Params: rollupBucketSeconds, startEpoch, endEpoch.
SELECT epoch/? AS b,
       COALESCE(SUM(strike_count), 0),
       MIN(CASE WHEN strike_count > 0 AND strike_dist_km > 0 THEN strike_dist_km END),
       MAX(CASE WHEN strike_count > 0 AND strike_dist_km > 0 THEN strike_dist_km END),
       COUNT(*)
FROM obs_st
WHERE epoch BETWEEN ? AND ?
GROUP BY b
ORDER BY b
