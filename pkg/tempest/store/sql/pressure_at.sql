-- The most recent station pressure at or before the given epoch, for the
-- barometric tendency: pairs with the latest reading to measure the change over
-- the trailing window. Skips NULL pressure so a sensor gap doesn't read as the
-- past sample. Params: epoch.
SELECT epoch, pressure_mb
FROM obs_st
WHERE epoch <= ? AND pressure_mb IS NOT NULL
ORDER BY epoch DESC
LIMIT 1
