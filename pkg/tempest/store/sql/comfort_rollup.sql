-- Per-bucket comfort rollup: one scan groups observations into fixed 15-minute
-- epoch buckets (see rollupBucketSeconds), and Go turns each bucket's mean
-- temperature, humidity, and wind into an apparent ("feels like") temperature and
-- dew point, then rolls the extremes up to local calendar days. Averaging over 15
-- minutes keeps the three inputs contemporaneous (a bucket's warmth, humidity, and
-- wind belong to the same quarter hour), which a whole-day average would not.
-- Params: rollupBucketSeconds, startEpoch, endEpoch.
SELECT epoch/? AS b,
       AVG(air_temp_c), AVG(humidity), AVG(wind_avg),
       COUNT(air_temp_c)
FROM obs_st
WHERE epoch BETWEEN ? AND ?
GROUP BY b
ORDER BY b
