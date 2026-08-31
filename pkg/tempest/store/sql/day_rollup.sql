-- Integer-bucketed pre-aggregation for the calendar queries: one scan groups
-- observations into fixed epoch buckets (15 minutes; see rollupBucketSeconds),
-- and Go assigns each whole bucket to a local calendar day. Every real-world
-- UTC offset (and DST shift) is a multiple of 15 minutes, so a bucket never
-- straddles a local day boundary, which keeps SQLite's per-row date functions
-- (the slow path: ~6µs/row in the pure-Go driver) out of full-table scans.
-- Params: rollupBucketSeconds, startEpoch, endEpoch.
SELECT epoch/? AS b,
       MIN(air_temp_c), MAX(air_temp_c),
       COALESCE(SUM(air_temp_c), 0), COUNT(air_temp_c),
       COALESCE(SUM(rain_mm), 0), MAX(wind_gust), COUNT(*)
FROM obs_st
WHERE epoch BETWEEN ? AND ?
GROUP BY b
ORDER BY b
