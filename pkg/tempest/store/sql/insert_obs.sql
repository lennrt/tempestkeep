-- Append one observation, columns in schema order. INSERT OR IGNORE turns a
-- repeat (device_id, epoch) into a no-op, so backfills and syncs are safely
-- repeatable. source is fixed to 'rest': every row here came from the REST API.
INSERT OR IGNORE INTO obs_st
  (device_id, epoch, wind_lull, wind_avg, wind_gust, wind_dir, wind_interval,
   pressure_mb, air_temp_c, humidity, illuminance_lux, uv, solar_wm2,
   rain_mm, precip_type, strike_dist_km, strike_count, battery_v,
   report_interval_min, source)
  VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'rest')
