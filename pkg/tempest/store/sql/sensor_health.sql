-- One-scan per-sensor health: the reading count and the epoch of the most recent
-- non-null value for each continuous sensor, plus the total row count and the
-- battery voltage range. A sensor whose last reading trails the archive's newest
-- observation has gone dark; a low reading count over the range means it has been
-- flaky. The event sensors (rain, lightning) are excluded here: a null there
-- usually means "nothing happened", not "sensor down".
-- Params: startEpoch, endEpoch.
SELECT COUNT(*),
       COUNT(air_temp_c),      MAX(CASE WHEN air_temp_c      IS NOT NULL THEN epoch END),
       COUNT(humidity),        MAX(CASE WHEN humidity        IS NOT NULL THEN epoch END),
       COUNT(pressure_mb),     MAX(CASE WHEN pressure_mb     IS NOT NULL THEN epoch END),
       COUNT(wind_avg),        MAX(CASE WHEN wind_avg        IS NOT NULL THEN epoch END),
       COUNT(uv),              MAX(CASE WHEN uv              IS NOT NULL THEN epoch END),
       COUNT(solar_wm2),       MAX(CASE WHEN solar_wm2       IS NOT NULL THEN epoch END),
       COUNT(illuminance_lux), MAX(CASE WHEN illuminance_lux IS NOT NULL THEN epoch END),
       COUNT(battery_v), MIN(battery_v), MAX(battery_v),
       MAX(epoch)
FROM obs_st
WHERE epoch BETWEEN ? AND ?
