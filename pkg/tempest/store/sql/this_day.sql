-- One local calendar day's aggregates for this-day-in-history: an indexed
-- range aggregate over that day, never a full-table scan.
-- Params: startEpoch (inclusive), endEpoch (exclusive).
SELECT MIN(air_temp_c), MAX(air_temp_c),
       COALESCE(SUM(air_temp_c), 0), COUNT(air_temp_c),
       COALESCE(SUM(rain_mm), 0), MAX(wind_gust), COUNT(*)
FROM obs_st
WHERE epoch >= ? AND epoch < ?
