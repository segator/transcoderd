# Proposal

## Why

Queue stop correctly holds prefetched encodes, but the worker counts them as active indefinitely and cannot currently prove that it is safe to restart or shut down. Local recovery can also repeat a completed download and the server can time out an assignment left idle for too long.

## What Changes

- Keep prefetched jobs waiting without encoding when the queue is stopped; finish already-running encodes and their remaining job phases.
- Track actual running work and in-flight reservations separately from parked prefetched jobs, and report safe idle only after recoverable state has been persisted.
- Preserve parked assignments, workspace and their next phase across safe restart, shutdown and applicable upgrades; on queue resume, continue the same jobs before requesting new work.
- Expose a safe-idle wait for operators and supervisors and display running and prefetched work distinctly.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-dispatch`: distinguish stopped, recoverable prefetched jobs from executing jobs and provide a safe-idle gate.
- `maintenance-and-recovery`: preserve parked assignments and restore their next phase without repeating completed work.
- `operations-dashboard`: display running work, parked prefetched jobs and safe-idle state independently.
- `windows-desktop-worker`: safe shutdown and manual update can retain parked prefetched jobs.
- `self-update`: an update of a stopped worker preserves parked work rather than starting its encodes.
- `events-and-progress`: accept worker events only for the current assignment and make event retries safe.
- `media-transfer`: fence transfer staging and final publication by assignment, including cancellation races.

## Impact

Worker scheduling, persistence, worker/server protocol, server assignment maintenance, update and shutdown coordination, dashboard telemetry, operational documentation and tests. An existing queue stop remains a stop, not a request to encode prefetched jobs.
