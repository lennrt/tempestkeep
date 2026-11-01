# ADR 0017: Historical rain sample coverage

Status: Proposed

## Context
Historical summaries reported zero when every rain measurement was missing. Rain normals counted
such months as dry.

## Decision
DayStat, PeriodStat and YearDay RainIn become *float64, encoded as a number or explicit null.
They retain Obs and expose RainReadings. MonthNormal AvgRainIn and AvgRainyDays become *float64
and RainYears counts contributing months across years. Missing-rain months do not enter rain
averages. RainyDays remains a count of observed rainy days.

## Consequences
Go clients must handle pointers; JSON clients must handle null. Existing archives and SI storage
remain unchanged. Partial readings produce available sums. Terminal summaries mark partial sums
with ≥; JSON carries the exact sample counts. Counts never establish temporal completeness and
partial months remain eligible for normals. Temperature normal denominators remain unchanged.
