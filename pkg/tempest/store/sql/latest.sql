-- Most recent observation. The Columns template value is the shared obs
-- SELECT list (obsColumns in store.go), in the exact order scanObs expects.
SELECT {{.Columns}} FROM obs_st ORDER BY epoch DESC LIMIT 1
