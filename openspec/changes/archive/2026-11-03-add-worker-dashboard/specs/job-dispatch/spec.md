# Spec Delta

## MODIFIED Requirements

### Requirement: Runtime worker admission controls
A worker SHALL offer local controls to report admission, job and encode counts and set non-negative job limits without restart. Positive limits SHALL update admission and FFmpeg concurrency. Zero SHALL stop requests and preserve the previous positive encode target. Lower limits SHALL let active encodes finish before another starts. The worker listener SHALL remain local. Authenticated dashboard controls SHALL persist desired limits per worker.

#### Scenario: Read runtime admission status
- **WHEN** an operator runs the local status command
- **THEN** the worker reports whether admission is enabled, active and reserved jobs, the job maximum, active and target encodes, and whether the schedule permits requests

#### Scenario: Disable new job requests
- **WHEN** an operator sets the maximum to zero
- **THEN** the worker stops new requests and leaves accepted and restored jobs running, including their later FFmpeg phase at the previous positive encode target

#### Scenario: A request is already in flight
- **WHEN** an operator sets the maximum to zero while a reserved request is waiting for the server
- **THEN** the worker reports that reservation and does not start further requests; the server may still assign the reserved job

#### Scenario: Change the maximum at runtime
- **WHEN** an operator sets a positive maximum different from the current value
- **THEN** the worker applies it to future job requests and FFmpeg encode concurrency without restarting

#### Scenario: Lower the encode maximum without interruption
- **WHEN** an operator lowers the maximum below the number of FFmpeg jobs already running
- **THEN** those encodes continue and no new encode starts until active encodes fall below the new target

#### Scenario: Reject invalid maximum
- **WHEN** an operator supplies a negative or non-integer maximum
- **THEN** the command reports an error and leaves the current maximum unchanged

## ADDED Requirements

### Requirement: Synchronize server-desired worker capacity
A worker SHALL resolve its persisted per-name job limit before initial admission and before subsequent admission evaluations, including while paused. An absent override SHALL preserve startup or current local configuration. Failed synchronization SHALL prevent new job requests for that cycle while accepted work continues. The normal synchronization interval SHALL be approximately five seconds.

#### Scenario: Remote positive change
- **WHEN** an operator changes a worker's persisted limit to a positive value
- **THEN** its next successful synchronization applies the admission and encode target without restarting or interrupting active encodes

#### Scenario: Resume a paused worker
- **WHEN** the worker is paused at zero and its persisted limit becomes positive
- **THEN** it continues polling and resumes admission after synchronization if its schedule permits

#### Scenario: Server unavailable
- **WHEN** settings synchronization fails
- **THEN** the worker skips new requests, keeps accepted work running, and retries on a later coordination cycle

#### Scenario: Automatic update drain
- **WHEN** an update causes the worker to drain active jobs
- **THEN** the temporary zero remains local and synchronization does not reopen admission during that drain
