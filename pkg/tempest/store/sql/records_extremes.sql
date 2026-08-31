-- Every scalar all-time extreme gathered in a single full scan; epoch_of.sql
-- recovers each extreme's timestamp with a cheap equality lookup afterwards.
SELECT MAX(air_temp_c), MIN(air_temp_c), MAX(wind_gust), SUM(strike_count),
       MIN(pressure_mb), MAX(solar_wm2), MAX(uv)
FROM obs_st
