## Decisions
Carry COUNT(rain_mm) through existing indexed and daily rollups. Return null when it is zero;
otherwise sum available readings, including measured zero. Retain obs and add rain_readings.
RainyDays counts only observed rainy days. Normals use a separate rain_years denominator;
temperature calculations remain independent.

Counts describe available samples, never complete temporal coverage. Partial months still
contribute available totals. Existing errors and range bounds remain unchanged.
