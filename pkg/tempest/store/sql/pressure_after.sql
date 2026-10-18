-- The first nonmissing pressure strictly after the target, bounded by the
-- latest epoch eligible for a distinct comparison. This forward index probe
-- complements pressure_at.sql; Go compares their distances and prefers the
-- older reading on ties. Params: targetEpoch, latestEligibleEpoch.
SELECT epoch, pressure_mb
FROM obs_st
WHERE epoch > ? AND epoch <= ? AND pressure_mb IS NOT NULL
ORDER BY epoch ASC
LIMIT 1
