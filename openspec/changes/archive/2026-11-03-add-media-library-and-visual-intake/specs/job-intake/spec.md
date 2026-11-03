# Spec Delta

## ADDED Requirements

### Requirement: Reviewed filtered batch intake
Dashboard batch intake SHALL accept only explicitly selected source files from a completed preview, revalidate fingerprints and current job state, and stop at the requested number of actual successful jobs. Existing work SHALL be skipped unless an applicable failed/canceled retry is explicitly enabled. Concurrent intake SHALL not create duplicate jobs in one server process. Per-file progress/results and cancellation SHALL be provided.

#### Scenario: Successful-job target
- **WHEN** a selected list contains skipped or failed candidates and a target of 500 successful jobs
- **THEN** loading continues through the reviewed selection until 500 jobs succeed or the selection ends, reporting actual successful and unsuccessful counts

#### Scenario: Source changed after preview
- **WHEN** a selected file changes or disappears before its enqueue attempt
- **THEN** that file is reported without queueing stale metadata or a different file

#### Scenario: Cancellation during loading
- **WHEN** an operator cancels an in-progress load
- **THEN** further submissions stop and already queued jobs remain queued
