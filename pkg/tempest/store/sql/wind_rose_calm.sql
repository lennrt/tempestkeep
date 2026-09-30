-- Count calm observations (wind data present but below the calm threshold),
-- which are reported as a share rather than binned, since their direction
-- carries no signal.
-- Include speed readings without a direction in the denominator.
-- Params: calmThresholdMps, startEpoch, endEpoch.
SELECT COUNT(CASE WHEN wind_avg < ? THEN 1 END), COUNT(*) FROM obs_st
WHERE epoch BETWEEN ? AND ? AND wind_avg IS NOT NULL
