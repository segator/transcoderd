# Proposal

## Why

The Windows dashboard currently selects one encoder for the entire worker, so users cannot use a GPU and CPU encoder concurrently on the same machine. Separate CPU and GPU worker profiles let operators use both resources while retaining one service and dashboard.

## What Changes

- Configure CPU and GPU encoder profiles and concurrency limits independently.
- Run both profiles under the existing Windows service and aggregate their status in the dashboard.
- Keep server queue assignment opportunistic: either profile may receive any queued job.
- Preserve existing saved single-encoder settings by migrating them to the matching profile.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `windows-desktop-worker`: support independently configured CPU and GPU worker profiles and aggregated monitoring.

## Impact

Affected areas include Windows desktop settings and dashboard, the Windows service supervisor and local control pipes, saved configuration migration, installer validation, and Windows desktop worker documentation. No server API changes are required.
