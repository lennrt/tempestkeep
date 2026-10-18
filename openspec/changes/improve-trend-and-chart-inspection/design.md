# Design

## Seasonal trend

Fit temperature = calendar-month intercept + common slope times year, with
equal weight per usable monthly mean. Subtract each calendar month's mean
from both time and temperature, then compute slope from centered cross-products.
A month observed only once has zero centered time and temperature and cannot
change the slope. Use centered values to avoid subtracting large year squares.
Keep anomaly extrema and coverage metadata based on the existing archive normals.

When centered time variance is zero, no slope is identifiable. When temperature
variance is zero but time variance is positive, the slope is zero and R² is
undefined, so omit R². Otherwise report the squared within-month correlation,
clamped to its mathematical range to absorb floating-point rounding.

## Chart inspection

Keep the existing bounded SVG rendering. Add a visible reading inspector using
the same source row and metric as the chart. Select the nearest returned point
by its actual plotted position; do not interpolate across gaps. Pointer and
touch choose a point, and keyboard controls allow first/last/previous/next
selection. Include the complete source timestamp and timezone, metric, units,
and an explicit missing value. No reading is invented in an absent bucket.

The controls must work at 390 pixels and for up to 2,000 points. Changing a
metric or replacing a result must reset or clamp selection without stale values.
Use text nodes for values and host-mediated lifecycle behavior unchanged.

## Verification and fallback

Use exact synthetic slopes, cooling, flat data, unmatched-month invariance,
unbalanced groups, no repeated months, and missing temperatures. Demonstrate
the original estimator fails the new regression. Browser tests cover dense
series, keyboard, pointer/touch, timestamps, missing versus zero, gaps, and
metric/result changes. Inspect screenshots. If chart interaction becomes too
large, retain keyboard/native range selection and defer pointer enhancements.
