# Proposal

## Why

Worker admission settings are fixed for the lifetime of a container, so operators must restart a worker to reduce load or stop it from taking more work. A runtime control command lets an operator preserve active encodes while changing the maximum number of jobs, with zero disabling requests for new jobs.

## What Changes

- Add local worker control commands to inspect status and change the maximum active-job count while the worker is running; setting the maximum to zero stops requests for new jobs.
- Apply changes to future job requests only; active jobs continue and a lowered limit takes effect as those jobs finish.
- Keep the worker's startup default at two concurrent jobs and preserve any configured schedule as an additional admission constraint.
- Provide a container-friendly invocation that works through `docker exec` without publishing a management port.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-dispatch`: define runtime-adjustable admission state and its effect on new assignments.

## Impact

- Worker admission logic and coordinator in `worker/worker/`.
- Worker CLI startup and a loopback-only local control interface in `worker/main.go` and related packages.
- Worker control documentation and `openspec/CONFIGURATION.md`.
