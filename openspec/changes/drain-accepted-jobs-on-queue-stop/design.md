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
4. Preserve ownership in the server across long idle periods and restarts. Persist and acknowledge the parked state with the current worker/job assignment and exclude only confirmed parked assignments from ordinary inactivity timeout. Cancellation or explicit recovery revokes the old assignment generation before a new one is created. A stale generation cannot publish an output or change authoritative job state. Physical duplicate computation cannot be ruled out for a disconnected or paused process; short renewable execution leases and stopping FFmpeg on lease loss limit wasted work but are not a proof that two processes never overlap. An operator can recover permanently lost parked work after revoking its generation.
5. Expose a local cancellable, bounded safe-idle wait backed by a restart barrier. The barrier remains effective through the caller's restart or an explicit release, and serializes admission/resume and job starts with the idle decision; unexpected client loss must not accidentally reopen admission. Supervisors, Windows shutdown and applicable upgrades use this gate rather than requiring the total accepted-job count to be zero. When the queue was already stopped, updates must not start parked encodes; park state and workspace survive replacement. Resume clears parked status safely and gives those jobs priority over new requests, while normal capacity and existing holds still apply.
6. Surface parked versus executing jobs in local and fleet telemetry, and clarify that server-side ownership can remain assigned while the worker is safely idle. Maintain wire compatibility with old clients; do not infer safety from an older worker lacking the new signal.

### Assignment identity and idempotent requests

Store a monotonically increasing assignment generation and current owner per job in PostgreSQL. Provision a stable individual credential per worker and authenticate that identity before issuing or replaying its assignment capability; a claimed worker name and the shared fleet token alone cannot establish that identity. Issue a unique, non-guessable assignment capability over the authenticated connection, store only its hash server-side, and persist it with the worker context without logging it. Derive the capability with HMAC from a versioned server secret and the assignment identity so the same capability can be reproduced for an idempotent retry without storing plaintext. All server replicas and restarts must share that secret; retain prior key versions until their assignments are terminal, and fail closed if a required key is unavailable. Generation is a fencing value, not a secret and not interchangeable with the existing per-event sequence number. Require the current generation and capability on protected job event, progress, download, upload and completion requests; serialize mutations for the job using a database row lock or equivalent compare-and-swap. Missing, revoked or stale credentials fail closed before any authoritative side effect. Use TLS when crossing untrusted networks.

Give each job request a worker-generated idempotency key persisted before sending. In the same database transaction, bind that key to one result, whether an assignment and generation or no job, and return that result on retry or after a lost response. A new poll uses a new key, so a remembered no-job response does not prevent later work. Reconcile unresolved keys after a worker restart before reporting safe idle. The server must arbitrate atomically across replicas, rather than relying on process-local mutexes.

### Park, restart and resume protocol

Close admission and hold an explicit restart barrier before observing idle. Resolve in-flight assignment requests; let in-progress preparations complete and active encodes finish through upload. At the pre-encode boundary, persist an atomic checkpoint with job ID, generation, next phase, encoder profile, source path, size and checksum; flush the file and platform-appropriate directory metadata. Persist a recoverable park intent before sending an authenticated, idempotent park request. The server transaction validates the current generation and commits the parked state. Only after the worker confirms that acknowledgement, verifies the checkpoint and configured restart-persistent workspace, and atomically observes no active phases, transfers, suspended processes or unresolved requests may it report safe idle. Failed writes or unknown acknowledgement remain non-idle and retry/reconcile; never treat missing data as a clean job.

On startup, restore settings with admission and encode starts closed. Read all contexts, reconcile pending events and park intents against the server, validate generation and local data, and restore parked jobs without redownloading when intact. Resume first atomically claims each current parked assignment, starts its queued encode within existing capacity/holds, and only then permits further assignments. A new worker process claims a new generation when resuming an assignment, fencing an older process with the same worker identity; it persists an idempotent claim key before the request and the resulting capability before starting work, so a crash or lost response can recover the same claim. If local storage is lost or corrupt, report an operator-visible recovery state; an explicit release increments the generation before requeueing. A `hostPath` only survives recreation on the same host and path; an operator-configured persistent-workspace policy and a stable volume identity are required to advertise safe restart, since probing a directory cannot prove that an ephemeral volume will survive.

### Event and output publication transaction

Persist each outgoing notification locally before sending. The server deduplicates retries by job, generation and event ID, acknowledging an identical committed event and rejecting a conflicting payload or stale generation. The worker records the acknowledgement before advancing, so a lost response or crash does not skip or duplicate history.

Stream uploads to isolated, immutable attempt paths scoped by job and generation. A short transaction locks the assignment, rechecks its generation and state at the moment of accepting the uploaded object, and records its path, size and checksum; a revoked generation cannot install a staged object. Completion first validates the event and generation, then durably moves the job to a `publishing` state with an immutable publication intent. While publishing is unresolved, cancellation and reassignment wait or return a conflict. Idempotent recovery completes or explicitly rolls back that intent after server crashes; only that generation's staged object can become the final output. Persist completion before optional source deletion, and retry deletion separately. PostgreSQL and filesystem renames are not a single transaction, so a mere check followed by a rename is insufficient.

### Rollout and compatibility

Advertise a new protocol capability. Deploy server migrations and protected handlers first, provision individual worker credentials and shared versioned server keys, then upgrade workers and supervisors. Legacy workers can continue legacy jobs but cannot park jobs or access a generation-protected job through older uncredentialed routes; unknown capability/status never implies safe idle. Existing contexts must be reconciled with the server before acquiring the new generation, and a parked context is never silently converted to an ordinary timeout assignment. Keep rollback explicit: never allow a downgraded server or old worker to bypass generation checks for an already protected assignment.

## Risks / Trade-offs

- An indefinitely parked assignment intentionally stays owned; provide explicit cancel/recovery for missing workspaces and do not silently reassign it.
- A worker crash, race or lost server acknowledgement can leave an ambiguous park transition. Reconcile durable worker/server state and fail closed until ownership is known.
- Existing worker events do not carry an assignment generation, and upload authorization checks only the worker name before streaming; an upload can be published after cancellation or reassignment. The protocol and recoverable publication state above must be implemented together before the safe-idle gate can be enabled.
- Network partitions can leave an old FFmpeg process consuming resources temporarily. Fencing protects server state and publication; renewable leases reduce redundant computation but cannot guarantee immediate physical termination on an unreachable host.
- Older workers and supervisors do not expose the safe-idle signal; keep their current conservative behavior until upgraded.
- A RAM-suspended active encode or slow upload prevents safe idle until the operator resumes it or it finishes; do not force it to run or cancel it.

## Migration Plan

Deploy the server's compatible parked-assignment support before relying on upgraded workers and supervisors. Existing snapshots use compatible recovery; the gate remains false until durable parked state is confirmed. Rollback must preserve assignment ownership and must not requeue parked jobs while an older worker may still resume them.
