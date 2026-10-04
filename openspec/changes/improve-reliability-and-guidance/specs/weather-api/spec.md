## ADDED Requirements

### Requirement: Cache only accepted responses

The API client MUST NOT cache malformed JSON or a response that fails envelope validation.

#### Scenario: A malformed response is followed by a valid response

- **WHEN** the caller retries the same endpoint
- **THEN** the client fetches the new response instead of replaying the invalid cache entry

### Requirement: Validate observation identity

The client MUST reject an explicit observation type other than obs_st and an
explicit response device ID that differs from the requested device.

#### Scenario: Another device type returns a different row layout

- **WHEN** the response declares obs_air or obs_sky
- **THEN** the client rejects it instead of storing those columns as Tempest observations

### Requirement: Preserve API failure status

The client MUST treat a non-success API status envelope as an error rather than empty history.

#### Scenario: An HTTP success contains an API failure

- **WHEN** the JSON status reports failure without observations
- **THEN** collection fails without marking the archive as exhausted

### Requirement: Reject missing result collections

The client MUST require an object response with the endpoint's expected collection
field. Explicit empty arrays and null collections MUST remain compatible.

#### Scenario: A successful HTTP response omits observations

- **WHEN** the JSON object lacks the obs field
- **THEN** the client rejects the response instead of recording empty history

### Requirement: Bound collections before typed decoding

The client MUST enforce endpoint array counts and nested device or observation
row limits before decoding those arrays into typed slices.

#### Scenario: A small JSON response contains too many empty station objects

- **WHEN** the stations array exceeds the station limit
- **THEN** the client rejects it before allocating a slice of decoded stations

### Requirement: Overflow-safe retry hints

Retry-After integer hints MUST remain bounded by the configured maximum wait,
including values that exceed the time.Duration range.

#### Scenario: A response contains a very large Retry-After value

- **WHEN** the server requests a delay larger than the supported duration
- **THEN** the client uses the maximum allowed wait without integer overflow
