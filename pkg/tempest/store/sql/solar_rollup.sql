-- Per-bucket solar rollup: one scan groups observations into fixed 15-minute
-- epoch buckets (see rollupBucketSeconds), and Go merges the buckets into local
-- calendar days. Estimate insolation using each sample's reported interval;
-- absent or nonpositive intervals cannot establish duration. The upper bound
-- matches model.DeviceObs validation. Maxima retain readings without intervals.
-- Params: rollupBucketSeconds, startEpoch, endEpoch.
SELECT epoch/? AS b,
       SUM(CASE WHEN report_interval_min > 0 AND report_interval_min <= 1440
                THEN solar_wm2 * report_interval_min * 60 / 1000000.0 END),
       MAX(solar_wm2), MAX(uv), MAX(illuminance_lux)
FROM obs_st
WHERE epoch BETWEEN ? AND ?
GROUP BY b
ORDER BY b
