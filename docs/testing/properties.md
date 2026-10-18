# Correctness properties

This catalog converts repository guarantees into test targets. A passing test is
evidence for the tested case. It is not proof of the general property.

All commands require Go 1.27.2. Tests have finite timeouts. Focused fuzz tests
store any failing input in the Go fuzz corpus so the failure can be replayed.

| ID | Property | Faults and boundaries | Current verification |
|---|---|---|---|
| P-01 | One archive accepts observations from only one device. | Two writers race to bind different devices; legacy rows disagree with metadata. | `TestConcurrentWritersCannotClaimDifferentDevices`, `TestWriterRejectsSecondDevice`, `TestOpenRejectsMixedDeviceArchive` |
| P-02 | Replaying the same observation does not add a duplicate or alter the first row. | Duplicate chunks, repeated collection, and restart from an earlier cursor. | `TestWriterInsertIdempotentAndReadParity`, `TestBackfillRangeChunksAndIsIdempotent` |
| P-03 | An invalid observation batch changes no archive state. | Invalid field after valid fields; oversized batch; invalid device. | `TestInsertObsRejectsInvalidBatchAtomically` and model validation tests |
| P-04 | A committed collection chunk remains visible after a later failure. | Fetch failure, progress-sink failure, cancellation, and restart. | `TestBackfillRangeResumesAfterError`, `TestProgressFailureReportsCommittedResumePoint`, `TestCollectSeedInterruptedResumes` |
| P-05 | A collection window is bounded, ordered, contiguous, and does not overlap its neighbor. | Minimum window, five-day limit, operation budget, and epoch boundaries. | Collector chunk tests and API window tests |
| P-06 | One `Backfiller` runs at most one operation at a time. | Concurrent `Sync`, forward fill, and backward fill calls. | `TestBackfillerRejectsConcurrentOperations`; `make race` |
| P-07 | Cancellation stops a request, retry wait, throttle wait, or query by its deadline. | Dependency outage and long configured waits. | `TestRetryStopsOnCancellation`, `TestBackfillerCancellationStopsThrottle`, context tests; `make race` |
| P-08 | Retry attempts and waits stay within the configured bounds. | HTTP 408, HTTP 429, HTTP 5xx, transport failure, `Retry-After`, and cancellation. | API retry tests |
| P-09 | Diagnostic errors do not contain a token, URL, response body, station ID, device ID, query, or local path. | Transport errors, malformed responses, HTTP failures, configuration I/O, store I/O, and cleanup errors. | API, configuration, and store redaction tests; `TestRunSanitizesTransportErrors` |
| P-10 | External input obeys documented size and work limits. | Oversized HTTP body, response counts before typed decoding, dotenv file, SQL values, rows, columns, metadata, observation batch, and backup directory. | `TestResponseCollectionsAreBoundedBeforeTypedDecode`, `TestQueryBoundsSQLiteValuesAndRestoresConnection`, limit tests, fuzz tests, and lint review |
| P-11 | The read handle cannot mutate the archive. | Direct DML, CTE-prefixed DML, multiple statements, and validator bypass attempts. | `TestQueryReadOnly`, `TestQueryRejectsWrites`, and SQLite `query_only` |
| P-12 | Caller-owned retained data is copied. | Mutation of an `obs_st` row or station device slice after a call. | `TestDeviceObsFromRowCopiesAndValidates`, `TestPickTempestDevice` |
| P-13 | `Close` is safe to repeat. The zero value behaves as closed. | Nil receiver, repeated calls, and use after close. | Store and writer close tests; `make race` |
| P-14 | With other writers stopped, a completed checkpoint permits a private backup that never overwrites an existing snapshot. | A reader pins the WAL, name collision, source or directory symlink, copy failure, permissive directory, and unrelated backup files. | `TestCheckpointRejectsBusyReader`, `collect_backup_test.go` |
| P-15 | MCP stdout contains protocol traffic only. Diagnostics contain no secret or local archive path. | Startup failures, live failures, write failures, and timezone warnings. | MCP protocol tests, stderr review, and secret scan |
| P-16 | Generated public API evidence matches the source. | Added, removed, or changed exported declarations and JSON tags. | `make generated` |
| P-17 | The production graph builds without cgo on the host and Linux ARM64. | Native dependency introduction and architecture-specific code. | `make build-pure build-arm64` |
| P-18 | A live dashboard cancels dependent work after a required fetch fails, reports optional forecast failure, and announces archive fallback. | Missing observations, stalled forecast, configured archive failure, and concurrent station resolution. | `TestNowLoadCancelsForecastAfterObservationFailure`, `TestNowLoadReportsOptionalForecastFailure`, `TestResolveNowConfigRejectsUnavailableConfiguredArchive`, `TestNowLoadAnnouncesArchiveFallback`; `make race` |
| P-19 | The default API transport rejects insecure TLS and never follows a redirect. | Old protocols, CBC-only servers, untrusted certificates, weak certificate keys, alternate verified chains, and every redirect status. | `TestDefaultTransportTLS`, `TestCertificateKeySizes`, `TestDefaultClientDoesNotFollowRedirects` |
| P-20 | MCP cancellation closes the archive, and tool registration reflects live, archive, and read-only settings. | SIGINT, SIGTERM, context cancellation with an open session, explicit client precedence, and token compatibility. | `TestE2ECommandMCPStdio`, `TestRunOverInMemoryTransport` |
| P-21 | Rejected API responses do not enter the cache or become empty collection results. | Failed API status, missing collection, duplicate and case-folded keys, wrong device/type, invalid sensor values, and timestamps outside the request. | `response_validation_test.go`, `TestDeviceObservationsRejectsWrongResponseIdentityAndRange`, `FuzzEndpointResponses` |
| P-22 | MCP backfill respects its work cap and preserves progress across calls. | One-day budgets, long empty gaps, restarts, explicit floors, stale empty-span state, and failed chunks. | `TestBackfillSmallBudgetsTerminateAcrossRestarts`, `TestBackfillExplicitFloorCrossesLongEmptyGap`, `TestBackfillFailurePreservesCommittedCursor` |
| P-23 | A collector resumes seed intent without a watermark and accepts a terminal cursor at zero. | First fetch failure, no observations yet, and failure after reaching the first supported epoch. | `TestCollectPersistsSeedIntentBeforeFirstFetch`, `TestCollectResumesSeedBeforeAnyObservation`, `TestCollectResumesTerminalSeedCursor` |
| P-24 | Series limits count aligned buckets, including partial buckets. | An unaligned start/end range, requested bucket width, and an exact point limit. | `TestAutoBucketRespectsInclusiveAlignedPointCap`, `TestRequestedBucketChecksItsOwnAlignment`, `TestObservationToolAcceptsHardCapBoundary` |
| P-25 | MCP output identifies the requested station and actual time range. | Non-default forecast station, explicit start without end, and daylight-saving transitions. | `TestForecastUsesRequestedStation`, `TestDailySummaryStartWithoutEndStopsNow`, `TestDailySummaryIncludesWholeDSTDay` |
| P-26 | Invalid CLI input fails before external work. | Extra arguments, malformed dates, future dates, and duration overflow. | `TestCommandsRejectPositionalArgumentsBeforeConfiguration`, `TestCollectRejectsInvalidDateBeforeDiscovery`, `TestNowRejectsOverflowingInterval` |
| P-27 | One MCP server serializes archive write calls and cancels waiting calls. | A second write waits behind an active call and then loses its context. | `TestBackfillQueuedCallCancelsWithoutFetching`, `make race` |
| P-28 | Solar energy estimates use each sample's positive report interval and retain sensor peaks without duration. | Sparse samples, differing intervals, absent or zero duration, reported zero irradiance, and intervals crossing a query boundary. | `TestSolarActivityUsesReportedIntervals`, `TestSolarActivitySparseAndDenseIntervals`, `TestSolarActivityMissingIntervalsRetainPeaks`, `TestSolarActivityAssignsReportedIntervalByTimestamp` |
| P-29 | Wind sectors retain fractional headings and use separate coverage denominators. | Fractional sector boundaries, legacy wrapped headings, missing direction, and missing speed. | `TestWindRosePreservesFractionalDirections`, `TestWindRoseCountsWindWithoutDirection` |
| P-30 | Missing event readings break dry and storm-free spells. | Consecutive observed days whose middle day has no rain or strike reading. | `TestMissingEventReadingsBreakSpells` |
| P-31 | Apparent temperatures require the sensors relevant to their temperature band. | Missing humidity in hot air, missing wind in cold air, mild air, and valid zero readings. | `TestComfortStatisticsRequiresRelevantSensor` |
| P-32 | Documented query examples stay valid and execute through MCP. | JSON syntax, named tool arguments, UTC month boundaries, missing readings, stored and display units, and gust ranking. | `TestDocumentationJSONExamples`, `TestIntegrationDocumentedQueries` |
| P-33 | Bounded MCP sync crosses empty windows while preserving retries and late arrivals. | Restarts, API errors, checkpoint failures, cancellation, changed watermarks, and delayed observations after a completed scan. | `TestSyncCrossesEmptyBudgetAcrossCalls`, `TestSyncCursorSurvivesRestartAndAPIError`, `TestSyncDiscardsCursorAfterWatermarkChanges`, `TestSyncReplaysWindowWhenCheckpointFails`, `TestSyncCancellationReplaysUncommittedWindow`, `TestSyncCompletedScanRetriesLateArrivingObservations` |
| P-34 | Terminal displays retain valid sensors and calendar boundaries, and setup preserves usable configuration on failure. | Quarter-hour timezones, daylight-saving changes, missing temperatures, empty ranges, terminal controls, literal PowerShell paths, and oversized dotenv output. | `TestDayChartRetainsQuarterHourTimezoneBuckets`, `TestDaysInCountsCalendarDaysAcrossSpringForward`, `TestWeekViewKeepsRainWhenTemperatureIsMissing`, `TestStatsEmptyAnalysisRangeDoesNotClaimNoLightning`, `TestListDevicesTextSanitizesAPILabels`, `TestPowerShellCommandArgPreservesLiteralPath`, `TestWriteEnvFileRejectsOversizedOutputBeforeReplacement` |
| P-35 | Live identity labels an archive only when its device belongs to that station; current pressure trends require recent readings and use a normalized three-hour rate. | Matched, mismatched, and unknown identities; future pressure; the one-hour freshness boundary; fresh rows without pressure; and sparse six-hour samples. | `TestNowFallbackRequiresMatchingArchiveIdentity`, `TestCrossSourceEnrichmentRequiresMatchingDevice`, `TestCurrentConditionsArchivePressureFreshnessAndRate` |
| P-36 | Explicit CLI repair is bounded, replay-safe, and preserves ordinary progress. | DST, 366-day bounds, invalid input before I/O, partial failure, duplicate replay, backups, and subprocess interruption. | `TestExplicitCollectionRangeCalendarBounds`, `TestCollectRangeRejectsInvalidInputBeforeIO`, `TestCollectRangeRepairsGapAndReplaysAfterFailure`, `TestCollectRangeCancellationKeepsCommittedRows` |
| P-37 | Explicit MCP continuation consumes an exact exclusive cursor without changing shared state. | Non-midnight DST boundary, invalid input before discovery, stored and absent metadata, and partial-failure replay. | `TestBackfillExactContinuationAcrossDST`, `TestBackfillExactContinuationPreservesSharedState`, `TestBackfillExactContinuationRejectsInvalidBeforeIO`, `TestBackfillExactContinuationReplaysAfterPartialFailure` |
| P-38 | Observation event totals carry independent sensor coverage. | Missing, measured-zero, partial and nonzero samples, clipped ranges, combined coverage/resolution notes, and legacy/SQL app compatibility. | `TestSeriesEventSensorCoverage`, `TestObservationSeriesReportsEventCoverage`, `npm run app:test` |
| P-39 | Seasonal trends depend on within-month change rather than unrelated seasonal coverage. | Exact warming/cooling, unmatched months, uneven groups, equal monthly weights, flat readings, absent repeated months, and large years. | `TestTemperatureTrendUnmatchedMonthCannotDiluteSlope`, `TestTemperatureTrendCentersUnevenMonthGroups`, `TestTemperatureTrendWeightsMonthlyMeansEqually`, `TestTemperatureTrendFlatRepeatedMonths`, `TestTemperatureTrendStableNearSupportedYearLimit`, `TestWriteStatsFlatTrend` |
| P-40 | Chart inspection selects returned readings without interpolation or extra tool calls. | Dense 2,000-point data, irregular time spacing, keyboard, touch, missing and zero readings, changed results, and resize selection/focus. | `npm run app:test`, `npm run app:screenshots` |
| P-41 | Optional export metadata preserves source values without changing default output. | CSV/JSONL, SI/US, exact solar arithmetic, missing/zero/fractional metadata, source controls, and failed writes. | `TestExportMetadataReproducesSolarEstimate`, `TestDefaultExportShapeRemainsStable`, `TestExportMetadataOutputFailureAndCancellation` |
| P-42 | Archive metadata streams through read-only bounded queries. | Legacy and uppercase columns, inclusive ordering, source byte/UTF-8 bounds, nonfinite metadata, cancellation, callback failure, and released connections. | `TestEachArchiveObsMetadataAndBounds`, `TestEachArchiveObsLegacySchema`, `TestEachArchiveObsSourceBounds`, `TestEachArchiveObsStopsAndClosesRows`, `TestEachArchiveObsRejectsInvalidOperations`, `TestEachArchiveObsRejectsNonFiniteMetadata` |
| P-43 | Compatible refreshes retain table exploration while displaying fresh rows. | Search/sort/page, changed data, page clamping, pending edits and focus, dataset reorder/removal, SQL duplicate columns, new contexts, and failed refreshes. | `tests/mcp-app/refresh.spec.mjs`, `npm run app:test` |
| P-44 | MCP collection and default live reads use the selected accessible Tempest device. | Flag/environment precedence, lazy startup, second stations, multiple devices per station, overrides, unknown and non-Tempest IDs, failed retries without writes, archive identity, offline reads, cancellation, and concurrency. | `TestE2EMCPDeviceConfigurationPrecedence`, `TestMCPSelectedDeviceControlsDefaultStationAndCollection`, `TestMCPDeviceSelectionFailuresDoNotWriteAndCanRetry`, `TestMCPSelectedDevicePreservesArchiveIdentityAndOfflineReads`, `TestSelectedDeviceResolutionCancellationAndConcurrency` |
| P-45 | Device inspection preserves parent stations and identifies bounded or imprecise values. | Exact ID/serial search, duplicates, missing and hostile labels, unsafe numeric IDs, stable empty refresh, traversal bounds, and unchanged raw and SQL results. | `tests/mcp-app/devices.spec.mjs`, `npm run app:test` |
| P-46 | Pressure tendency selects the nearest eligible sample without a full-history distance sort. | Reversed-direction regression, predecessor/successor/tie, exact and half windows, short histories, one-second and odd windows, early epochs, missing pressure, cancellation, and classified errors. | `TestPressureTendencyNearestEligibleSample`, `TestPressureTendencyNearEpochStart`, `TestPressureTendencyMissingAndErrors`, `TestPressureSampleQueriesUseEpochIndex`, `TestArchivePressureUsesNearestEligibleReading` |
| P-47 | Reports reject unsupported ranges before configuration or archive access. | Malformed dotenv, absent archive paths, epoch and elapsed-span bounds, timezone offsets, and 23/25-hour days. | `TestReportRangesRejectUnsupportedBoundsBeforeIO`, `TestReportRangeSupportedBoundaries`, `TestReportRangePreservesDSTCalendarDays` |
| P-48 | Rain totals distinguish missing readings, zero, and partial samples. | Empty and all-missing ranges, mixed zero/positive values, inclusive clipping, DST days, missing-day spells, cancellation, and closed stores. | `TestRainStatsJSONCoverage`, `TestRainStatsCoverageUsesInclusiveSelectedRange`, `TestRainStatsCoverageAcrossDST`, `TestRainStatsCoveragePreservesErrors` |
| P-49 | CLI and MCP preserve nullable totals and sample counts. | Actual command JSON/text, unrelated weather sections, nullable required MCP schema, plain/structured results, and bounded date selection. | `TestE2EStatsRainCoverage`, `TestRainStatsSchemaAndCoverage` |
| P-50 | The rainfall app keeps missing totals visible without implying dry weather. | Empty/missing/partial/zero, coverage notices, legacy payloads, raw-data preservation, unrelated SQL, and refresh. | `tests/mcp-app/rain-summary.spec.mjs`, `npm run app:test` |

## Test commands

```sh
make test
make integration
make e2e
make race
make fuzz
make generated
```

To replay one regression from the repository root, name its package and give
the test a timeout:

```sh
go test ./pkg/tempest/store -run '^TestCheckpointRejectsBusyReader$' -count=1 -timeout=30s
```

Go prints and stores the input for a fuzz failure. Before committing a minimized
input, make sure that it contains no token, payload, personal data, or raw identifier.

These properties describe specific boundaries. SQL result limits do not bound
all temporary SQLite memory. A completed checkpoint does not prevent another
process from writing during a later file copy. An empty-history stop does not
prove that no older observations exist beyond a long upstream outage.

## Live smoke test

A live smoke test is optional locally and is not a required CI check. Load a
short-lived token into the process environment from a secret manager. Then run:

```sh
make live-smoke
```

The script discards API output. It creates an owner-only temporary archive,
checks one bounded history range and offline replay, and removes the archive on
exit. Do not save station names, coordinates, serial numbers, device IDs, or
token-bearing URLs as evidence.

The live smoke test does not establish durability, performance, service
availability, or production readiness.
