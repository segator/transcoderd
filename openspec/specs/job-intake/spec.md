# Job intake

## Purpose

Define how a request for one file or a directory becomes persisted jobs, how destination paths are derived, and when an existing job may be queued again.

## Requirements

### Requirement: Job submission API
The server SHALL accept `POST /api/v1/job/` with a JSON `sourcePath` and SHALL return a JSON result with `scheduled`, `failed`, and `skipped` collections.

#### Scenario: Missing source path
- **WHEN** `sourcePath` is absent or empty
- **THEN** the API returns HTTP 400

#### Scenario: Submission processed
- **WHEN** the scheduler processes the requested path
- **THEN** the API returns HTTP 200 with per-file results

### Requirement: File discovery
The scheduler SHALL walk the requested path beneath `server.scheduler.sourcePath` and evaluate files against the configured minimum byte size and supported extension list. A directory request SHALL allow multiple files to be evaluated in one response.

#### Scenario: Directory with eligible media
- **WHEN** a requested directory contains files with supported extensions and sufficient size
- **THEN** the scheduler attempts to create a job for each eligible file

#### Scenario: File too small
- **WHEN** a file is smaller than `server.scheduler.minFileSize`
- **THEN** the file and its reason appear in `failed`

#### Scenario: Unsupported extension
- **WHEN** a file's extension is not supported
- **THEN** the file and its reason appear in `failed`

### Requirement: New job identity and state
A new job SHALL receive a UUID, a source path relative to the configured media root, a relative destination path, the source size, and an initial `Job/queued` event.

#### Scenario: New eligible file
- **WHEN** an eligible source has no existing job
- **THEN** the server stores a new job and its first queued event

### Requirement: Destination naming
The scheduler SHALL generate a `.mkv` destination path, replacing recognized video codec labels with `x265` and recognized audio codec labels with `AAC`. If that generated path equals the source path, it SHALL add `_encoded.mkv` to avoid using the same file as input and output.

#### Scenario: Codec labels in the name
- **WHEN** the source path is `shows/episode.x264.dts.avi`
- **THEN** the generated destination path is `shows/episode.x265.AAC.mkv`

#### Scenario: Name otherwise unchanged
- **WHEN** the source path is `movies/film.mkv` and no codec label changes
- **THEN** the destination path is `movies/film_encoded.mkv`

### Requirement: Requeue an existing job
A persisted job SHALL return to `queued` through a user request only when the request provides the force flag matching its `completed`, `failed`, or `canceled` state. Internal timeout recovery is specified in `maintenance-and-recovery`.

#### Scenario: Completed job forced
- **WHEN** an existing completed source is submitted with `forceCompleted: true`
- **THEN** the same job receives a new `Job/queued` event

#### Scenario: Existing job without applicable force
- **WHEN** an existing source is submitted without a flag applicable to its state
- **THEN** the request reports that file in `failed` without creating a second job

### Requirement: Cancel assigned work
The server SHALL accept `DELETE /api/v1/job/{jobid}` and SHALL append a `Job/canceled` event with a user-cancellation reason when the identified job is `assigned` or `started`.

#### Scenario: Started job
- **WHEN** a cancellation request names a started job
- **THEN** a `Job/canceled` event is recorded with a user-cancellation reason

#### Scenario: Completed job
- **WHEN** a cancellation request names an already completed job
- **THEN** the server returns an error without appending a cancellation event
