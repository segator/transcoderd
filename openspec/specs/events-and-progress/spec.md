# Events and progress

## Purpose

Define the worker-to-server event protocol, durable job history, finalization behavior, and temporary progress reporting.

## Requirements

### Requirement: Event envelope
The server SHALL accept `POST /api/v1/event` containing `eventType` and `eventData`. It SHALL recognize `Ping`, `Notification`, and `Progress` event types, and SHALL reject an unknown event type.

#### Scenario: Valid presence event
- **WHEN** a valid `Ping` event is submitted
- **THEN** the worker's presence is updated and the API returns HTTP 200

#### Scenario: Unknown event type
- **WHEN** an event type other than the three recognized values is submitted
- **THEN** the API returns an error

### Requirement: Sequential notification history

A job notification SHALL carry a phase type, status, worker name, message, and integer `eventID`. The server SHALL append a new event only when its `eventID` immediately follows the last stored event for that job. An identical replay SHALL be acknowledged without duplicating its effects. A conflicting event SHALL receive an explicit definitive rejection.

#### Scenario: Next notification
- **WHEN** a notification uses the next expected event number
- **THEN** the server stores it and updates the job's current status projection

#### Scenario: Exact replay after a lost response
- **WHEN** the worker retries an event already committed with the same identity and payload
- **THEN** the server acknowledges it without appending or repeating finalization

#### Scenario: Duplicate or skipped number
- **WHEN** a notification conflicts with a stored event or skips ahead
- **THEN** the server rejects it definitively without appending it to the history

### Requirement: Phase and job status reporting
The worker SHALL report `started` and `completed` notifications for each executed phase. A failed action SHALL generate a failed phase notification and a `Job/failed` notification containing the error.

#### Scenario: Encoding phase succeeds
- **WHEN** the FFmpeg action completes
- **THEN** the worker reports `FFMPEG/completed` and advances to output verification

#### Scenario: Encoding phase fails
- **WHEN** the FFmpeg action fails
- **THEN** the worker reports `FFMPEG/failed` and `Job/failed`, then cleans the job workspace

### Requirement: Finalize a completed job
Before recording `Job/completed`, the server SHALL inspect the uploaded output's primary video stream, choose a non-conflicting codec-derived destination, and persist the final destination path and size. If probing or finalization fails, the server SHALL NOT record the job as completed or delete the source. If `deleteOnComplete` is enabled, the server SHALL attempt source deletion only after finalization and completion-event persistence succeed.

#### Scenario: Destination exists
- **WHEN** the worker reports `Job/completed` and the uploaded destination can be probed
- **THEN** the server chooses a free codec-derived destination without overwriting existing files and records its path and size before storing the completion event

#### Scenario: Output verification fails
- **WHEN** the server cannot probe the uploaded output or it contains no video stream
- **THEN** the server rejects completion and preserves the source file

#### Scenario: Source deletion enabled
- **WHEN** `deleteOnComplete` is enabled and finalization plus completion-event persistence succeed
- **THEN** the server attempts to remove the source file

#### Scenario: Source deletion disabled
- **WHEN** `deleteOnComplete` is disabled
- **THEN** the server preserves the source file after successful finalization

### Requirement: Temporary progress
A `Progress` event in state `progressing` SHALL create or update a progress entry keyed by progress ID and phase. A `done` or `failure` state SHALL remove that entry.

#### Scenario: Phase in progress
- **WHEN** the server receives a progressing event with percentage and ETA
- **THEN** it stores the current progress value and update time

#### Scenario: Phase ends
- **WHEN** the server receives a done or failure event for that progress ID and phase
- **THEN** the active progress entry is removed

### Requirement: Worker progress frequency
The worker SHALL update its local console during actions and SHALL publish server progress no more often than approximately every 15 seconds while a phase progresses, with a final done or failure event when the action ends.

#### Scenario: Frequent local updates
- **WHEN** an action emits multiple local updates within 15 seconds
- **THEN** the worker does not publish a server progress event for every update

#### Scenario: Action ends
- **WHEN** the tracked action completes or fails
- **THEN** the worker publishes a final progress state

### Requirement: Local current task progress
The worker SHALL expose a local snapshot of active task identifiers, phases, percentage and estimated remaining time through its existing authenticated local control channel.

#### Scenario: Query active task
- **WHEN** a local authorized client requests status while a task is progressing
- **THEN** the response includes its current progress without waiting for the next server event

#### Scenario: Task finishes
- **WHEN** a task completes or fails
- **THEN** it is removed from the current-task snapshot

### Requirement: Local pipeline queue snapshot
The worker SHALL report accepted jobs waiting or running in each pipeline phase and whether their source has been downloaded. The local snapshot SHALL include jobs held outside the channel while waiting for an encode slot.

#### Scenario: Prefetched encode waits for capacity
- **WHEN** a downloaded job is queued for FFmpeg while all encode slots are occupied
- **THEN** local status reports the job as queued, downloaded and waiting for FFmpeg

### Requirement: Optional worker hardware telemetry
Workers SHALL publish optional timestamped CPU/RAM and visible NVIDIA device readings through existing runtime heartbeats. Collection SHALL be read-only, default to five-second sampling and bound GPU queries to two seconds/two MiB without blocking heartbeat or job execution. CPU percentage SHALL require counter deltas. Unknown/unsupported sensors SHALL be unavailable rather than zero, with host or processor-group CPU scope explicit.

#### Scenario: Supported worker
- **WHEN** a worker can read system counters and NVIDIA telemetry
- **THEN** its heartbeat and dashboard API include measured CPU usage, memory and available per-device GPU/encoder/decoder utilization, VRAM, temperature and power

#### Scenario: CPU warm-up or GPU failure
- **WHEN** a first CPU reading has no prior sample or a GPU query is absent, unsupported, fails or times out
- **THEN** those measurements are unavailable while other telemetry and job coordination continue

#### Scenario: Older worker
- **WHEN** a worker omits resource telemetry
- **THEN** presence, runtime controls and job handling remain compatible and hardware readings are not fabricated

#### Scenario: Worker stops
- **WHEN** worker execution shuts down
- **THEN** its sampler and any active GPU query are canceled

### Requirement: Resilient notification delivery

The worker SHALL persist pending job notifications and retry transient network errors, request timeouts and server failures indefinitely without discarding accepted work. Each attempt SHALL have a bounded duration. Notification delivery for one job SHALL NOT block unrelated jobs. An ambiguous response SHALL NOT be classified as definitive rejection.

#### Scenario: Server temporarily unavailable
- **WHEN** the server is unreachable or returns a transient failure
- **THEN** the worker retains the job and retries its unchanged pending notification until acknowledged or shutdown

#### Scenario: Worker restarts with an uncertain notification
- **WHEN** a worker restarts after sending a notification whose acknowledgement was not received
- **THEN** it recovers and retries the same notification without discarding the job

#### Scenario: Definitive rejection
- **WHEN** the server explicitly rejects a notification because the assignment expired
- **THEN** the worker stops that job, records the reason, discards its workspace, and continues processing other jobs

#### Scenario: Explicit notification conflict
- **WHEN** the server explicitly rejects a notification because its sequence or payload conflicts with recorded history
- **THEN** the worker stops that job, records the rejection, preserves its workspace for inspection, and releases its execution slot

#### Scenario: Interrupted transfers
- **WHEN** a source download, checksum request, or upload fails because the server or network is temporarily unavailable
- **THEN** the worker retries without a fixed overall retry deadline and preserves the accepted job

### Requirement: Fence reassigned jobs

Updated workers SHALL identify the assignment on job notifications and media transfers. The server SHALL reject a stale assignment before accepting its notification or publishing its upload. A job reassigned to the same worker SHALL have a different assignment identity.

#### Scenario: Upload finishes after reassignment
- **WHEN** an upload started under an assignment that has since expired
- **THEN** the server rejects its publication and the worker discards the expired local job

#### Scenario: Same worker receives a later assignment
- **WHEN** a prior execution sends an event or upload after the job was reassigned to the same worker name
- **THEN** the server rejects the old assignment without affecting the new one
