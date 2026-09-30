-- Bin non-calm wind observations into 16 compass sectors.
-- Sector index = direction rounded to the nearest 22.5° step, mod 16: the
-- SQL twin of model.Compass. Normalize with subtraction because SQLite's %
-- casts both operands to integers and would discard fractional directions.
-- Params: startEpoch, endEpoch, calmThresholdMps.
WITH directions AS (
    SELECT wind_dir - 360 * CAST(wind_dir / 360 AS INTEGER) AS direction,
           wind_avg, wind_gust
    FROM obs_st
    WHERE epoch BETWEEN ? AND ?
      AND wind_dir IS NOT NULL AND wind_avg IS NOT NULL AND wind_avg >= ?
)
SELECT CAST((CASE WHEN direction < 0 THEN direction + 360 ELSE direction END)
            / 22.5 + 0.5 AS INTEGER) % 16 AS sector,
       COUNT(*), AVG(wind_avg), MAX(wind_gust)
FROM directions
GROUP BY sector
