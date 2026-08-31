-- Downsample observations into fixed buckets aligned to the unix epoch.
-- Aggregation per field matches its physics: temperatures keep avg/min/max,
-- gust and UV keep the max, rain and lightning strikes sum. Buckets with no
-- observations are absent rather than zero-filled.
-- Params: bucketSeconds, bucketSeconds, startEpoch, endEpoch.
SELECT (epoch/?)*? AS b,
       AVG(air_temp_c), MIN(air_temp_c), MAX(air_temp_c),
       AVG(humidity), AVG(pressure_mb), AVG(wind_avg), MAX(wind_gust),
       COALESCE(SUM(rain_mm), 0), MAX(uv), AVG(solar_wm2),
       COALESCE(SUM(strike_count), 0), COUNT(*)
FROM obs_st
WHERE epoch BETWEEN ? AND ?
GROUP BY b
ORDER BY b
