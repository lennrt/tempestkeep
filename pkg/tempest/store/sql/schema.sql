-- The archive schema. Every TempestKeep writer (the collector CLI and the MCP
-- archive tools) creates and appends to this same obs_st table, so one file
-- serves them all. IF NOT EXISTS makes opening an existing archive a no-op.
CREATE TABLE IF NOT EXISTS obs_st (
  device_id            INTEGER NOT NULL,
  epoch                INTEGER NOT NULL,   -- unix seconds (observation time, UTC)
  wind_lull            REAL,
  wind_avg             REAL,
  wind_gust            REAL,
  wind_dir             INTEGER,
  wind_interval        INTEGER,
  pressure_mb          REAL,
  air_temp_c           REAL,
  humidity             REAL,
  illuminance_lux      REAL,
  uv                   REAL,
  solar_wm2            REAL,
  rain_mm              REAL,
  precip_type          INTEGER,
  strike_dist_km       REAL,
  strike_count         INTEGER,
  battery_v            REAL,
  report_interval_min  INTEGER,
  source               TEXT DEFAULT 'rest',
  PRIMARY KEY (device_id, epoch)
);
CREATE INDEX IF NOT EXISTS idx_obs_epoch ON obs_st(epoch);
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT);
