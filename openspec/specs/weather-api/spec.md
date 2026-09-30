# Weather API access

## Purpose

Read WeatherFlow station data through bounded HTTP requests and retain useful
error classifications without exposing credentials in diagnostics.

## Requirements

### Requirement: Bounded HTTP responses

The client MUST reject responses larger than its configured maximum body size.

#### Scenario: The service sends an oversized response

- **WHEN** an HTTP response exceeds the body limit
- **THEN** the client returns a classified error without retaining the full body

### Requirement: Bounded retries

The client MUST bound retry attempts and wait times by its retry policy.
Cancellation MUST stop retry waits and external work.

#### Scenario: A request is canceled during a retry delay

- **WHEN** the request context is canceled
- **THEN** the request returns a cancellation error without another HTTP attempt

### Requirement: Redacted diagnostics

Client errors MUST omit access tokens, raw response bodies, and sensitive URL values.

#### Scenario: A transport error includes a credential-bearing URL

- **WHEN** the client returns the failure to its caller
- **THEN** the error retains its supported classification without the URL values
