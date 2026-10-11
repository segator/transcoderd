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
On `Job/completed`, the server SHALL inspect the destination file, persist its size, and attempt to remove the source if `deleteOnComplete` is enabled.

#### Scenario: Destination exists
- **WHEN** the worker reports `Job/completed` and the destination exists
- **THEN** the destination size is recorded for the job

#### Scenario: Source deletion enabled
- **WHEN** `deleteOnComplete` is enabled after a destination is found
- **THEN** the server attempts to remove the source file

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
