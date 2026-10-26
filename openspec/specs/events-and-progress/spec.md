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
A job notification SHALL carry a phase type, status, worker name, message, and integer `eventID`. The server SHALL store it only when its `eventID` immediately follows the last stored event for that job.

#### Scenario: Next notification
- **WHEN** a notification uses the next expected event number
- **THEN** the server stores it and updates the job's current status projection

#### Scenario: Duplicate or skipped number
- **WHEN** a notification repeats an event number or skips ahead
- **THEN** the server rejects it without appending it to the history

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
