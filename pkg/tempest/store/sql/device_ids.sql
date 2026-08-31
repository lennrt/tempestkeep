-- At most two ids are needed to detect an archive whose aggregate analytics
-- would mix readings from different weather stations.
SELECT DISTINCT device_id
FROM obs_st
ORDER BY device_id
LIMIT 2;
