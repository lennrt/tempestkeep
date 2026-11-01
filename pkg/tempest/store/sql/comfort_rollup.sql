-- Quarter-hour comfort means. Both members of each derived-input pair use
-- exactly the same rows; independent means would combine unpaired sensors.
-- The temperature-only mean retains the mild-air fallback.
-- Params: rollupBucketSeconds, startEpoch, endEpoch.
SELECT epoch/? AS b,
       AVG(air_temp_c),
       AVG(CASE WHEN humidity IS NOT NULL THEN air_temp_c END),
       AVG(CASE WHEN air_temp_c IS NOT NULL THEN humidity END),
       AVG(CASE WHEN wind_avg IS NOT NULL THEN air_temp_c END),
       AVG(CASE WHEN air_temp_c IS NOT NULL THEN wind_avg END)
FROM obs_st
WHERE epoch BETWEEN ? AND ?
GROUP BY b
ORDER BY b
