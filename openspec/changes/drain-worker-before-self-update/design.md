# Design

## Context

The update supervisor launches the service as a child process. The worker coordinator checks the release before each job request and currently exits the child as soon as an update is found. JobExecutor already tracks active jobs and provides a runtime admission limit.

## Goals / Non-Goals

**Goals:**
- Prevent a worker's self-update from interrupting jobs already assigned to it.
- Ensure an update-restart exit causes the supervisor to fetch the latest release immediately.

**Non-Goals:**
- Change server update behavior, release cadence, or the 15-minute normal update-check interval.
- Persist active jobs across a worker process replacement.

## Decisions

- On update detection, release the reserved slot, set the worker's active-job limit to zero, and wait for the existing active-job count to reach zero before exiting with the supervisor's update code. This reuses current admission and job-lifecycle accounting rather than adding another queue or restart protocol.
- Make the supervisor invalidate its cached check after the child returns the update code. This ensures it downloads the new asset instead of relaunching the old binary until the normal check interval elapses.
- Keep heartbeats running while draining so the server continues to see the worker as alive. Do not add a drain timeout: abandoning active work after a timeout would recreate the failure this change prevents.

## Risks / Trade-offs

- A stuck job can delay an update indefinitely → retain existing job timeouts and allow the operator to stop the worker explicitly if necessary.
- The update waits for all jobs on that worker, even when one has already completed → this is intentional because the binary process is shared by all concurrent jobs.

## Migration Plan

No configuration or data migration is needed. Once released, workers drain active jobs before applying future releases.
