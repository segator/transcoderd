# Design

## Context

The coordinator polls the server every five seconds and currently checks admission before making a request. The worker has no external runtime control surface, and its concurrency limit is a constant. See the proposal and `specs/job-dispatch/spec.md` for motivation and the behavior contract.

## Goals / Non-Goals

**Goals:**
- Provide process-local operator controls usable with `docker exec`.
- Make admission and capacity checks race-safe with a job request already in flight.
- Preserve active jobs when the maximum is set to zero or reduced.
- Keep management access local to the worker container.

**Non-Goals:**
- Add a server-side API or distributed configuration store.
- Stop, cancel, or reprioritize active jobs.
- Freeze and resume a running FFmpeg process; the control command changes admission only.
- Change the worker update mechanism or encoder configuration.

## Decisions

- Use a Unix domain socket at `/run/transcoderd-worker/control.sock`, in a `0700` directory with socket mode `0600`, rather than a published TCP port. The worker image already contains its own binary, so operators can invoke `transcoderd-worker control <command>` through `docker exec` without host port mapping or extra tools. Keeping the socket under `/run` avoids exposing it through a commonly mounted worker `/tmp` volume.
- Support `status` and `set-max-jobs <non-negative integer>`. Zero disables new job requests; a positive value enables admission up to that limit. The default maximum remains two and is configurable at startup; runtime updates live only for the current worker process.
- Protect the maximum and in-flight reservations with one mutex. The coordinator reserves capacity before requesting a server assignment and releases it if the request fails or returns no job. This prevents a concurrent control update from silently allowing a request that exceeds the new limit, except for a request already reserved before the setting changed.
- Count resumed persisted jobs as active before admitting new jobs. Keep at least one encoder consumer available when the startup admission maximum is zero so persisted jobs can continue. Admission checks always apply the configured time window and current concurrency maximum together.
- The control client sends one JSON request per socket connection and receives a status/error response. No network listener is created.

Alternatives considered: a TCP admin port requires port mapping and expands network exposure; signal-based control does not naturally support arbitrary numeric limits or status; editing a mounted file needs polling and persists operator state across restarts unintentionally.

## Risks / Trade-offs

- [The socket path can collide if multiple worker processes share one filesystem namespace] → Containers have separate `/tmp`; on a shared host filesystem, only one worker process can use the default socket at a time, and startup fails clearly rather than taking over another process's socket.
- [A job request already reserved before a limit change can still be assigned] → Reservations count toward capacity and are included in status; the command does not cancel a request already sent to the server.
- [Runtime settings reset on restart] → Startup defaults and configuration remain the source of initial state; document that controls affect the running process only.

## Migration Plan

No data migration is needed. Existing workers start with a maximum of two active jobs unless configured otherwise. Deploy the worker image, then use `docker exec <container> /app/transcoderd-worker control ...` to inspect and update the running worker. Rollback by deploying the previous image; no server change is required.
