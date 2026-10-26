# Design

## Context

The worker already has a local Unix-socket control CLI and identifies itself to the server with `worker.name`. The authenticated worker client uses the server's Bearer token. PostgreSQL's `workers` table is presence data keyed by worker name, so durable desired settings should not be coupled to its IP/last-seen lifecycle.

## Goals / Non-Goals

**Goals:**

- Persist the desired job maximum per stable worker name in server PostgreSQL.
- Apply runtime updates only after the server confirms persistence.
- Load the persisted value before starting job admission after a process restart.
- Use the configured startup value only until an operator saves a desired limit for that worker.
- Keep the human-operated control surface local to `docker exec` or `kubectl exec`.

**Non-Goals:**

- Remote server-initiated pause, resume, restart, or arbitrary command execution on workers.
- Persisting schedules, codecs, FFmpeg options, or other worker settings in this change.
- Changing deployment defaults or worker concurrency policy.

## Decisions

### Store a separate optional worker setting keyed by `worker.name`

Add a small `worker_settings` table with `worker_name` as its primary key, a non-negative `max_active_jobs`, and an update timestamp. A row means an operator has set the worker's desired limit; no row means use the worker's startup configuration. Setting zero stores a paused admission state. Operators resume by setting a positive limit. Keeping settings separate from the existing presence table avoids requiring IP and heartbeat fields for settings writes and avoids treating transient presence as configuration. The worker name is already the server's stable key for presence and job assignment; operators must keep it stable to retain the setting.

### Use authenticated worker-initiated read/write operations

Add narrowly scoped authenticated API operations to read the current worker's optional maximum and save a non-negative maximum. Every operation uses the existing Bearer token and the worker name supplied by the running worker client. The API does not send commands to workers. The local control CLI remains the only operator interface; `set-max-jobs N` persists first and then changes in-memory admission/encode targets. There is no delete/reset operation: zero pauses new assignments, and a positive value resumes them at the requested limit.

### Resolve the override before opening job admission

At startup, the worker reads its override before the coordinator can request jobs. A returned value, including zero, replaces the startup value. A missing row keeps the startup value. If the server is unavailable, the worker keeps restored jobs running but does not request new jobs until it can resolve the override; this prevents an unknown persisted zero or lower limit from being bypassed during an outage. The existing server client retry behavior can be reused, but the startup path must remain cancellable on process shutdown.

### Persist before applying runtime changes

For an operator's `set-max-jobs` control request, the worker asks the server to commit the setting before changing the in-memory target or returning success. On a server error, the local command fails and the current in-memory setting remains unchanged. The updater's internal drain-to-zero operation must remain process-local; otherwise an automatic update would permanently pause the worker in the database. This avoids reporting a temporary change as durable when a restart would undo it.

## Risks / Trade-offs

- [A server outage delays admission after worker restart] → Continue restored work, keep admission closed, and retry the settings read until the server is available.
- [Renaming a worker makes its prior setting appear absent] → Document `worker.name` as the durable settings key and retain the startup fallback when no row exists.
- [A shared Bearer token can authorize settings changes for any named worker] → Keep the API narrowly scoped to a job maximum and require the existing authenticated worker API; do not add server-to-worker control.
- [Database migration or API rollout may be incompatible with older servers] → Deploy the server migration/API before workers that require persisted settings, and keep the startup configured value as fallback when the API explicitly reports no override.

## Migration Plan

1. Deploy the server version containing the additive table migration and worker settings API.
2. Deploy workers with the new control behavior. Workers with no stored row continue using their existing startup configuration.
3. Operators use `control set-max-jobs N` to persist and apply a value; use zero to pause admission and a positive value to resume it.
4. Rollback workers first if needed. The additive table can remain unused; do not drop it as part of application rollback.
