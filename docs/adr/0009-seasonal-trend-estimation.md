# ADR 0009: Seasonal trend estimation

- Status: proposed
- Date: 2026-10-18

## Context

Subtracting calendar-month temperature means alone does not remove the effect
of uneven seasonal time coverage from a regression. The previous fit divides
by time variation between unrelated months and understates the within-month
trend. Its broad numerical test tolerated this error.

## Decision

Fit one slope with a separate intercept for each calendar month. Give each
usable monthly mean equal weight. Center both time and temperature within its
calendar-month group, then calculate the slope from centered cross-products.
Report R² for the within-month fit, not the seasonal level differences.

A calendar month observed once supplies no trend information. If no month
repeats, omit the slope and R². Repeated constant temperatures identify a zero
slope but no R² because temperature variance is zero. Keep existing anomaly
extrema and coverage metadata.

## Consequences

Corrected slopes and R² values can differ from previous reports. Constant
repeated readings now return zero slope instead of no slope. Field names,
JSON types, public Go signatures, and stored data remain unchanged.

This is a descriptive fit to the available archive. It does not establish
climate attribution, station stability, complete coverage, or statistical
significance. Sparse months receive the same weight as more complete months;
users must inspect coverage before interpreting the result.
