-- Integer-bucketed pre-aggregation for the hourly climatology (the diurnal
-- profile): one scan groups observations into fixed 15-minute epoch buckets, and
-- Go folds each whole bucket into its local hour-of-day. Like day_rollup.sql,
-- this leans on every real-world UTC offset (and DST shift) being a multiple of
-- 15 minutes, so a bucket never straddles a local hour boundary; the slow
-- per-row date functions stay out of the scan.
-- Params: rollupBucketSeconds, startEpoch, endEpoch.
SELECT epoch/? AS b,
       MIN(air_temp_c), MAX(air_temp_c),
       COALESCE(SUM(air_temp_c), 0), COUNT(air_temp_c),
       COALESCE(SUM(humidity), 0), COUNT(humidity),
       COALESCE(SUM(wind_avg), 0), COUNT(wind_avg),
       MAX(wind_gust), COUNT(*)
FROM obs_st
WHERE epoch BETWEEN ? AND ?
GROUP BY b
ORDER BY b
