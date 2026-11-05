# Proposal

## Why
Remote Pause both only changes admission limits and cannot suspend an active encode or restore the prior limits. The Workers period selector also fails to filter presence, and resource cards show percentages without local history.

## What Changes
- Separate RAM pause/resume from stop-after-current/resume-queue, with authenticated durable commands and worker acknowledgement.
- Preserve positive limits for queue resume across refresh/restart; preserve independent RAM and individual holds.
- Filter Workers membership by the selected 24h/7d/30d period, within the existing 30-day fleet maximum.
- Show bounded CPU/RAM/GPU resource plots directly in online worker cards, according to profile.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `operations-dashboard`: explicit remote controls with pending/applied/error feedback, period-filtered Workers view and compact hardware history.
- `job-dispatch`: remote controls reuse local pause/admission semantics and worker-owned process suspension.

## Impact
Additive PostgreSQL migration, worker settings/control wire payloads, repository/scheduler/API, worker polling, frontend controls/resource cards, tests and operational documentation. Upgrade server and worker for remote pause support; older workers remain readable but these actions require an update.
