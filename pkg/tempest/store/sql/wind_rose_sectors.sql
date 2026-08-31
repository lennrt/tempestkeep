-- Bin non-calm wind observations into 16 compass sectors.
-- Sector index = direction rounded to the nearest 22.5° step, mod 16: the
-- SQL twin of model.Compass. The double-mod normalizes any out-of-range
-- direction the way Compass does.
-- Params: startEpoch, endEpoch, calmThresholdMps.
SELECT CAST(((wind_dir % 360) + 360) % 360 / 22.5 + 0.5 AS INTEGER) % 16 AS sector,
       COUNT(*), AVG(wind_avg), MAX(wind_gust)
FROM obs_st
WHERE epoch BETWEEN ? AND ?
  AND wind_dir IS NOT NULL AND wind_avg IS NOT NULL AND wind_avg >= ?
GROUP BY sector
