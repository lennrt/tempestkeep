# Accurate trends and direct chart inspection

## Why

The temperature trend removes calendar-month means from temperatures but not
from time. Uneven month coverage therefore attenuates the fitted trend. A pair
of January readings warming by 3.6°F in one year reports about 0.897°F per
decade after an unrelated December is added, instead of 36°F per decade.
Dense charts also omit hover targets above 60 points, even though the default
observation request returns up to 288 points.

## What Changes

- Fit a common trend with separate calendar-month intercepts by centering both
  temperature and time within each calendar month.
- Report a zero slope for repeated constant readings; omit R² when anomaly
  variance is zero and omit the slope when there is no repeated month.
- Add pointer, touch, and keyboard inspection of all returned chart points.

## Capabilities

### Modified Capabilities

- `archive-storage`: seasonally adjusted trend estimation.
- `mcp-service`: accessible chart reading inspection.

## Impact

Trend numbers change intentionally; fields and storage stay the same.
The chart adds local controls without tool calls or dependencies. See
[ADR 0009](../../../docs/adr/0009-seasonal-trend-estimation.md).
