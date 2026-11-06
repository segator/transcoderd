# Proposal

## Why

Fleet telemetry exposes protocol capabilities but not the running worker release. Operators cannot identify which machines still need an upgrade from the dashboard.

## What Changes

- Workers publish their own running binary version with heartbeat telemetry.
- Worker cards, selected resource details and overview summaries show the reported version, including last-reported versions for offline workers.
- Older workers show Version unavailable; versions are never inferred from capabilities or the API release.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `operations-dashboard`: expose running worker version in authenticated fleet monitoring.

## Impact

Additive runtime JSON field, worker heartbeat construction, frontend presentation, documentation and regressions. Existing runtime JSON storage requires no migration or new endpoint. No version lookup or worker update is triggered by viewing the dashboard.
