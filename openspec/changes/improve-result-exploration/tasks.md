# Tasks

- [x] 1. Reproduce dataset focus loss, hidden long SQL values, and absent daily cards.
- [x] 2. Add local focus restoration, bounded SQL inspection/search, and daily summaries.
- [x] 3. Pass browser tests, bundle build/check, and strict OpenSpec validation.
- [x] 4. Inspect narrow and desktop UI screenshots.
- [x] 5. Independently review the integrated change (eight integrated browser cases passed).

## Verification evidence

Synthetic Chromium baseline checks showed focus moving to BODY, no daily summary
cards, and an unsearchable suffix beyond 2,048 characters. New browser regression
checks failed against the original bundle before implementation. Exact post-change
checks are recorded after execution.

Post-change: `npm run app:build`, `npm run app:check`, `npm run app:screenshots`,
and strict `npm run spec:check` passed. Five new browser checks passed. The first
full browser run passed 59 of 60 tests: the existing pointer geometry test also
failed against the untouched HEAD bundle in an isolated temporary fixture host.
Its geometry acquisition now polls for a connected target without fixed sleeps;
selection assertions remain unchanged. The final complete browser suite passed
60 of 60 tests. Desktop forecast and 390-pixel cell inspector screenshots were
inspected, along with the standard documentation captures. Third-party hosts
and screen-reader speech were not tested.
