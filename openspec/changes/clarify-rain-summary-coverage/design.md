## Problem and Evidence

Two missing readings currently produce `total_in: 0`. Two measured zeroes and
one zero plus one missing sample can produce identical complete RainStats JSON.
CLI text prints 0.00 inches whenever days exist. The app's generic scalar hero
would skip a nullable total and instead headline an unrelated numeric count.

## Chosen Contract

Add `Observations` and `RainReadings` integer fields, always encoded as
`observations` and `rain_readings`. Sum existing `dayAgg.obs` and `dayAgg.rainN`
from the same bounded read; no extra query, archive migration, or retained data.
Change `TotalIn` to `*float64` with `json:"total_in"`. It is nil exactly when no
rain readings exist. Otherwise it is the sum of reported rainfall in inches,
including a nonnil zero. Never fill missing readings or extrapolate a total.

Empty observations mean no observations. Positive observations with no rain
readings mean unknown rainfall. Fewer rain readings than observations mean a
partial reported total. Equal positive counts mean rain was supplied for every
stored observation, not continuous coverage of the requested period.

Keep DaysObserved as days with any observation. Preserve rainy-day threshold,
wettest-day behavior, and dry/wet spell rules; days without rain readings and
calendar gaps still end spells. Counts and spells describe recorded observations.

## Presentation and Compatibility

CLI text shows an explicit unavailable message for no rain readings. Available
totals include sample counts; partial data gets a clear warning. Other weather
sections remain available even when rain is missing. CLI JSON and MCP expose
explicit null, never omit the total. MCP schema must accept number and null.
MCP notes distinguish empty/missing/partial results without claiming uptime.

The app pins its rain_stats headline to rainfall even when null. Show sample
coverage and a missing/partial notice. If `rain_readings` explicitly equals zero,
suppress weather-statistic zeroes in the display while retaining
coverage counts and the original result. Older payloads without coverage counts
keep their prior numeric meaning. Arbitrary SQL and other tools are untouched.

An additive count-only design was considered, but it leaves misleading numeric
zero in canonical MCP and JSON outputs. A duplicate nullable total would add two
meanings for one quantity. Independent challenge favors the single nullable total
with a documented Go/JSON compatibility change in the unreleased minor version.
This intentionally revises RainStats shape stability in the earlier reliability
proposal; other aggregate result types remain unchanged.

## Scope and Verification

Implement store semantics, CLI/MCP output, scalar app behavior, and feature docs
as separate verified slices. Use synthetic empty/missing/zero/mixed fixtures,
inclusive range bounds, DST grouping, cancellation, and preserved spell rules.
Prove the old code fails a regression that inspects serialized output. Check MCP
output schema, text and structured content, CLI JSON, and browser refresh/layout.

Run focused tests, full make verify, API snapshot update, strict OpenSpec checks,
and app build/check/test/screenshots. Inspect desktop and narrow screenshots and
request independent diff review. A smaller fallback ships store and CLI/MCP
semantics with the scalar missing-total headline and docs; never keep a misleading
zero solely to avoid a consumer update. No live credentials are needed.
