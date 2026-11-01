# Use the Go security patch

## Why

Required vulnerability checks detect GO-2026-6809 in the Go 1.27.2 baseline.
Go 1.27.3 fixes the vulnerability.

## What Changes

- Pin active builds and verification to Go 1.27.3.
- Update current guidance without changing historical evidence.
- Retain all language dependency versions.

## Impact

Contributors need the patched toolchain. No archive migration or publication
is required. ADR 0020 records the security decision and official sources.
