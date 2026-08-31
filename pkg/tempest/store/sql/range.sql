-- Every stored observation in [start, end], oldest first: the export path's
-- scan. Shares the obs column list (see queries.go) so the SELECT order matches
-- what scanObs expects, exactly like latest.sql. Params: startEpoch, endEpoch.
SELECT {{.Columns}}
FROM obs_st
WHERE epoch BETWEEN ? AND ?
ORDER BY epoch
