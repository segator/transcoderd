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
The scheduler SHALL generate a provisional `.mkv` path, replacing recognized video and audio labels with `x265` and `AAC`, and add `_encoded` when it would equal the source. The server SHALL isolate each upload attempt, publish only complete checksum-validated data, support finalization across filesystems, probe the actual video codec, choose a non-conflicting codec-derived path, and persist it before recording `Job/completed`.

#### Scenario: Codec labels in the name
- **WHEN** the scheduler accepts the source path `shows/episode.x264.dts.avi`
- **THEN** the provisional upload destination path is `shows/episode.x265.AAC.mkv`

#### Scenario: H.265 and HEVC source labels are recognized
- **WHEN** the source path contains an `h265` or `hevc` label and the uploaded output's video codec is `av1`
- **THEN** the generated destination path replaces that label with `av1`

#### Scenario: Name otherwise unchanged
- **WHEN** the source path is `movies/film.mkv` and no video or audio codec label changes
- **THEN** the provisional upload destination path is `movies/film_encoded.mkv`

#### Scenario: H.264 source encoded to AV1
- **WHEN** the source path is `movies/film.x264.AC3.mkv` and the uploaded output's video codec is `av1`
- **THEN** the final destination path is `movies/film.av1.AAC.mkv`

#### Scenario: HEVC source encoded to AV1
- **WHEN** the source path is `movies/film.x265.AAC.mkv` and the uploaded output's video codec is `av1`
- **THEN** the final destination path is `movies/film.av1.AAC.mkv`

#### Scenario: AV1 source encoded to HEVC
- **WHEN** the source path is `movies/film.av1.AAC.mkv` and the uploaded output's video codec is `hevc`
- **THEN** the final destination path is `movies/film.x265.AAC.mkv`

#### Scenario: Output filename has no codec label
- **WHEN** the source path is `movies/film.mkv` and the uploaded output's video codec is `av1`
- **THEN** the final destination path is `movies/film.av1.mkv`

#### Scenario: Final path would overwrite an existing file
- **WHEN** the codec-derived final destination already exists and is not the source file
- **THEN** the server preserves the existing file and chooses the next available alternative name, such as `_encoded.mkv`, `_encoded-2.mkv`, or a higher numbered suffix, until it finds a free destination

#### Scenario: Output codec cannot be verified
- **WHEN** the server cannot probe the uploaded output or it contains no video stream
- **THEN** the job is not recorded as completed and the source file is not deleted

#### Scenario: Staging and library are on different filesystems
- **WHEN** an upload is staged on a filesystem different from the final media destination
- **THEN** the server transfers it to a destination-local staging file and finalizes it without overwriting an existing file
- **AND** the server records the final path and size before recording the job as completed

#### Scenario: A failed upload retry cannot remove a successful upload
- **WHEN** one upload attempt fails while another attempt for the same job succeeds
- **THEN** the failed attempt's cleanup removes only its own temporary file
- **AND** the complete, checksum-validated upload remains available for finalization

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
