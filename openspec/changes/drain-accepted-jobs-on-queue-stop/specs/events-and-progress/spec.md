## MODIFIED Requirements

### Requirement: Sequential notification history
A job notification SHALL carry a phase type, status, worker name, message, and integer `eventID`. For a protected assignment, the server SHALL verify its generation before effects, store the next event, acknowledge identical committed retries and reject stale generations or conflicting payloads. Legacy assignments retain sequential-event behavior.

#### Scenario: Next notification
- **WHEN** a notification uses the next expected event number
- **THEN** the server stores it and updates the job's current status projection

#### Scenario: Duplicate or skipped number
- **WHEN** a protected assignment retries an identical committed event, repeats a number with different data, or skips ahead
- **THEN** the server acknowledges the identical retry and rejects conflicting or skipped events without appending them

### Requirement: Finalize a completed job
Before recording `Job/completed`, the server SHALL validate the current assignment and durably authorize publication for protected jobs, then inspect the uploaded output's primary video stream, choose a non-conflicting codec-derived destination, and persist the final destination path and size. If probing or finalization fails, the server SHALL NOT record the job as completed or delete the source. If `deleteOnComplete` is enabled, the server SHALL attempt source deletion only after finalization and completion-event persistence succeed.

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

#### Scenario: Completion from an old assignment
- **WHEN** a revoked assignment sends a completion notification
- **THEN** the server rejects it before modifying job state, final output or source media

#### Scenario: Crash during finalization
- **WHEN** the server crashes after authorizing publication but before recording completion
- **THEN** recovery resolves the same publication intent without accepting a conflicting assignment or deleting the source prematurely
