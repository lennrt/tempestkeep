-- Per-bucket solar rollup: one scan groups observations into fixed 15-minute
-- epoch buckets (see rollupBucketSeconds), and Go merges the buckets into local
-- calendar days. The average irradiance per bucket lets Go integrate daily
-- insolation (a bucket's mean W/m² held over its 900 s), while the maxima carry
-- the peak instantaneous irradiance, UV index, and illuminance for the day.
-- Params: rollupBucketSeconds, startEpoch, endEpoch.
SELECT epoch/? AS b,
       AVG(solar_wm2), MAX(solar_wm2),
       MAX(uv), MAX(illuminance_lux),
       COUNT(solar_wm2)
FROM obs_st
WHERE epoch BETWEEN ? AND ?
GROUP BY b
ORDER BY b
