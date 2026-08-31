-- First epoch whose {{.Column}} equals the extreme value found by the
-- single-pass scan in records_extremes.sql. The equality lookup terminates at
-- the first match, so it is far cheaper than an ORDER BY over the whole table.
-- The column is an internal constant (never user input), so template
-- substitution is safe here.
SELECT epoch FROM obs_st WHERE {{.Column}} = ? LIMIT 1
