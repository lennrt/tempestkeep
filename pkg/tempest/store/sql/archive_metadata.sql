-- Only return known optional metadata names, keeping legacy schema discovery
-- bounded without retaining arbitrary column names from the archive.
SELECT lower(name) FROM pragma_table_info('obs_st')
WHERE lower(name) IN ('device_id', 'report_interval_min', 'wind_interval', 'precip_type', 'source')
