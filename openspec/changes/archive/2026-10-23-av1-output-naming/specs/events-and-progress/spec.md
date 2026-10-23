# Spec Delta

## MODIFIED Requirements

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
