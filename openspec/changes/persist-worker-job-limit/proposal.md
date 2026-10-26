# Proposal

## Why

Runtime `control set-max-jobs` changes only the current worker process, so a container restart silently restores the deployment value. Operators must repeat manual commands after every restart, and the server has no durable record of the intended per-worker capacity. Persisting this setting centrally will make worker restarts predictable while preserving a startup fallback for new workers.

## What Changes

- Persist an optional desired maximum job count for each named worker in the server database.
- Make runtime `set-max-jobs` save the desired value and apply it immediately; restore it before a restarted worker can request jobs.
- Treat the persisted value as the worker's desired maximum: zero prevents new assignments while allowing accepted jobs to finish, and a positive value resumes admission at that limit.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-dispatch`: runtime job-limit changes become durable per worker and take precedence over the startup fallback after restart.

## Impact

- Worker control socket/CLI and server client for persisting and loading worker settings.
- Authenticated worker settings API and server repository/scheduler.
- PostgreSQL worker settings migration and repository tests.
- Job-dispatch OpenSpec contract, protocol/configuration documentation, and worker deployment guidance.
- No new external dependency. Worker identity remains its configured `worker.name` and must be stable to retain its stored setting.
