# Design

## Context

Zero limits already close admission and hold queued FFmpeg starts. However, `activeJobs` counts all accepted jobs, including parked work, and shutdown/update paths currently wait for that count to reach zero. Local recovery enqueues the last recorded phase even if that phase completed. Server maintenance can requeue idle assignments after `jobTimeout` (default 24 hours).

## Goals / Non-Goals

**Goals:**
- Let running encodes and their verification/upload finish while prefetched jobs stay parked until the queue is explicitly resumed.
- Make restart or shutdown safe without losing or reassigning parked jobs, and allow an operator or supervisor to wait for this condition.
- Preserve existing independent RAM and per-job pause semantics; never silently resume a paused encode merely to claim idle.

**Non-Goals:**
- Restart a pod or host from this control, or preserve a running FFmpeg process across a restart.
- Treat a missing worker, an unconfirmed in-flight request, paused running encode, or an unpersisted parked context as safe idle.

## Decisions

1. Preserve queue-stop semantics: persist zero limits before application; stop new requests and queued FFmpeg starts. A job whose encode has started is allowed to finish through upload, while previously accepted pre-encode work may finish preparation and then parks before encoding. A request reserved before stop is resolved and, if assigned, retained in the same way. Explicit RAM or per-job holds remain independent and may prevent the active job from finishing.
2. Introduce synchronized activity accounting across queue transitions, reservations, phase execution, persistence and completion. Report executing jobs, reserved requests, parked prefetched jobs, and safe idle independently; accepted-job totals retain their existing meaning for compatibility. Safe idle requires admission closed, no executing phase or running/suspended encode, no unresolved request, and every parked job durably recoverable and acknowledged by the server. Take the decision atomically with respect to transitions and fail closed for unknown status or concurrent resume.
3. Persist a recoverable next phase and encoder profile at the park boundary before confirming safe idle. Use an atomic write and validate associated downloaded files on recovery. Resume the parked job at FFmpeg without repeating a completed download when its data remains intact; missing or corrupt context/data must be reported and must not be silently marked idle or completed. Existing persisted snapshots need a compatible recovery path. Workspace storage must survive the intended restart and remain accessible on the same worker; ephemeral storage or rescheduling elsewhere cannot be assumed safe.
4. Preserve ownership in the server across long idle periods and restarts. Persist and acknowledge the parked state with the worker/job assignment, exclude only confirmed parked assignments from ordinary inactivity timeout, and fence stale workers, resume, cancel and reassignment so a job cannot run twice. An explicit user cancellation must still work. Handle crashes between persistence and acknowledgement by reconciliation; never let a stale parked marker make a truly lost assignment unrecoverable without an explicit recovery path.
5. Expose a local cancellable, bounded safe-idle wait backed by a restart barrier. The barrier remains effective through the caller's restart or an explicit release, and serializes admission/resume and job starts with the idle decision; unexpected client loss must not accidentally reopen admission. Supervisors, Windows shutdown and applicable upgrades use this gate rather than requiring the total accepted-job count to be zero. When the queue was already stopped, updates must not start parked encodes; park state and workspace survive replacement. Resume clears parked status safely and gives those jobs priority over new requests, while normal capacity and existing holds still apply.
6. Surface parked versus executing jobs in local and fleet telemetry, and clarify that server-side ownership can remain assigned while the worker is safely idle. Maintain wire compatibility with old clients; do not infer safety from an older worker lacking the new signal.

## Risks / Trade-offs

- An indefinitely parked assignment intentionally stays owned; provide explicit cancel/recovery for missing workspaces and do not silently reassign it.
- A worker crash, race or lost server acknowledgement can leave an ambiguous park transition. Reconcile durable worker/server state and fail closed until ownership is known.
- Existing worker events do not carry an assignment generation, and upload authorization checks only the worker name before streaming; an upload can be published after cancellation or reassignment. A parked marker alone therefore cannot fence stale workers. The server and worker protocol must define and enforce a generation on event acceptance and final output publication, including in-flight operations and mixed-version clients, before the safe-idle gate can be enabled.
- Older workers and supervisors do not expose the safe-idle signal; keep their current conservative behavior until upgraded.
- A RAM-suspended active encode or slow upload prevents safe idle until the operator resumes it or it finishes; do not force it to run or cancel it.

## Migration Plan

Deploy the server's compatible parked-assignment support before relying on upgraded workers and supervisors. Existing snapshots use compatible recovery; the gate remains false until durable parked state is confirmed. Rollback must preserve assignment ownership and must not requeue parked jobs while an older worker may still resume them.
