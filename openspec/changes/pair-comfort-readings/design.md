# Design

SQL returns the temperature-only mean, paired temperature/humidity means, and
paired temperature/wind means per existing quarter-hour bucket. Each paired
mean uses the same row predicate for both members. Go evaluates warm and cold
candidates from their respective paired means, plus the existing mild-air
fallback. Daily extremes include all available candidates. Dew point uses only
the humidity pair. Missing pairs yield NULL means; zero values remain usable.

Regression fixtures use public Store and Writer operations with synthetic
observations. Test missing pairs, extra unpaired rows, distinct buckets, zeros,
and complete pairs. Keep cancellation and archive errors on the existing path.
