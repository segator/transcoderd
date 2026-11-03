# Proposal

## Why
The fleet dashboard must stay synchronized with the current cross-platform worker UI. Local independent limits and RAM/per-job holds are not yet exposed in fleet telemetry, and managed startup can overwrite a newer server setting.

## What Changes
- Integrate the latest worker dashboards, Windows service and queue/RAM pause behavior without losing independent encoders or media functionality.
- Add atomic CPU/GPU local web/CLI controls backed by server persistence; poll server settings while paused and retain holds during limit changes.
- Publish queue, RAM, individual job and maintenance hold states; display them distinctly in the server dashboard.
- Bootstrap managed settings only when absent; show observed Windows profile limits instead of stale saved form values.
- Preserve the current one-extra-pipeline-job policy and queued-encode pause contract from main.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `job-dispatch`: synchronized local pair controls and startup precedence while preserving pause/maintenance holds.
- `operations-dashboard`: truthful observed pause states and synchronized capacity forms.

## Impact
Worker control/client/runtime, local/shared worker UI, Windows service status, fleet telemetry/rendering, merged specs and operational docs. No production deployment or queue mutations are required. The PR includes all earlier dashboard, dual-encoder and media-library changes.
