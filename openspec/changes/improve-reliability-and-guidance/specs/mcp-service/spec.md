## ADDED Requirements

### Requirement: Accurate forecast station identity

Forecast output MUST label data with the station selected by the request.

#### Scenario: A caller selects a non-default station

- **WHEN** the station is accessible to the configured client
- **THEN** the forecast data and station label refer to that station

### Requirement: Match identities before combining live and archived weather

Station identity from live discovery MUST label an archive only when its device
belongs to that station. Live conditions MUST include an archive pressure trend
only when that identity matches. Unknown identity MUST omit enrichment.

#### Scenario: The configured archive belongs to another station

- **WHEN** station_info returns archive coverage
- **THEN** it does not attach the independently discovered live station's identity

### Requirement: Current pressure trends use recent readings and stated units

For either conditions source, the latest pressure reading MUST be no later than
the displayed observation and less than one hour older. The pressure_trend_3h_inhg
field MUST represent a normalized three-hour rate.

#### Scenario: Matching-device pressure history is stale

- **WHEN** the archive's latest pressure reading is at least one hour older than live data
- **THEN** current_conditions omits the historical pressure trend

#### Scenario: A recent archived row has no pressure

- **WHEN** older pressure readings would produce a stale trend
- **THEN** the recent row does not make that trend current

#### Scenario: Pressure samples span six hours

- **WHEN** the samples differ by six millibars
- **THEN** pressure_trend_3h_inhg represents three millibars per three hours, converted to inHg

### Requirement: Open-ended summaries stop at the present

A daily summary with an explicit start and no end MUST use the current time as
its upper bound. It MUST reject a start that lies after that bound.

#### Scenario: A caller omits the summary end date

- **WHEN** the server constructs the summary range
- **THEN** its upper bound is the current time rather than the following day

### Requirement: Hard backfill work limits

A backfill call MUST request no more history than max_days permits.
Continuation metadata MUST distinguish a work-budget stop from completed history.

#### Scenario: The budget is smaller than a normal chunk

- **WHEN** the caller sets max_days to one
- **THEN** the total requested history spans at most one day

### Requirement: Actionable capability guidance

Status messages, prompts, and resources MUST recommend operations that the server
can perform with its configured live client and archive access.

#### Scenario: An empty archive opens read-only

- **WHEN** a client requests archive status or a workflow prompt
- **THEN** guidance explains the missing capability instead of asking for unavailable write tools

### Requirement: Validate tool inputs before external work

Malformed date inputs MUST fail before live station discovery or collection begins.

#### Scenario: A backfill request contains an invalid date

- **WHEN** the server receives the request
- **THEN** it reports invalid input without making an HTTP request

### Requirement: Sync progress crosses empty work budgets

Bounded sync calls MUST preserve successful forward scan progress across calls
and restarts. The saved cursor MUST be tied to the current observation watermark.
Failed or uncommitted windows MUST remain eligible for replay.
After a complete sync, later calls MUST recheck the empty tail after the watermark
so a forward cursor does not skip late-arriving recent observations.

#### Scenario: An empty period is longer than one sync budget

- **WHEN** the caller repeats sync_archive after each bounded result
- **THEN** later calls eventually reach newer available observations

#### Scenario: Another writer advances the observation watermark

- **WHEN** sync loads a cursor anchored to the former watermark
- **THEN** it discards that cursor and resumes from the current stored observations

#### Scenario: A request fails after a successful empty window

- **WHEN** the caller retries sync
- **THEN** it retains committed progress and requests the failed window again

#### Scenario: The API publishes a recent observation after an empty complete scan

- **WHEN** a later sync runs with the same observation watermark
- **THEN** it requests the empty tail again and can retrieve the late observation
