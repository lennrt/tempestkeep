-- Count calm observations (wind data present but below the calm threshold),
-- which are reported as a share rather than binned, since their direction
-- carries no signal.
-- Params: startEpoch, endEpoch, calmThresholdMps.
SELECT COUNT(*) FROM obs_st
WHERE epoch BETWEEN ? AND ? AND wind_avg IS NOT NULL AND wind_avg < ?
