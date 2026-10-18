# ADR 0011: MCP device selection and pressure samples

Status: proposed

## Context

CLI collection can select a device but MCP always selects the first Tempest on
the account. This prevents consistent use of another station's archive. Pressure
tendency promises the nearest window sample but considers only earlier candidates,
which can reverse the reported pressure direction in sparse history.

## Decision

Add MCP `--device-id` and honor `TEMPEST_DEVICE_ID`. A positive explicit flag
wins. Empty or zero environment values preserve discovery. Validate configuration
before archive/API work, then resolve authorized ST membership lazily. Do not
cache failed resolution or fall back to a different live device. Preserve offline
archive access, explicit station read overrides, and existing archive identity
checks. Live requests remain station-level even when a device is selected.

Compare indexed pressure candidates on both sides of the requested start. Require
at least half the window between distinct samples, prefer the older sample on a
tie, and retain actual-span normalization. No schema or output-field changes are
needed. Invalid report ranges fail as usage errors before configuration or I/O.

## Consequences

Existing MCP configurations that set `TEMPEST_DEVICE_ID` now select that device.
Previously ignored malformed values become errors. Discovery remains the default
when the key is empty or zero. Startup can initialize an empty archive before
lazy device resolution, but failed resolution cannot bind or collect into it.
Pressure values can change when a closer eligible sample exists, and sufficiently
long short histories can now yield a trend. The station app adds a local derived
device table without changing MCP payloads or adding calls. Version 0.4.0 remains
the current development target; no publication or merge is included.
