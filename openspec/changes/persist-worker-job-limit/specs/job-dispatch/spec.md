# Spec Delta

## MODIFIED Requirements

### Requirement: Runtime worker admission controls
A running worker SHALL provide local controls to report admission, job, and encode counts and set the job maximum to any non-negative integer without restart. A positive value SHALL update admission and the FFmpeg encode target. Zero SHALL stop requests and preserve the previous positive encode target. Setting the maximum SHALL persist the worker's desired value on the server before reporting success. On startup, a persisted value SHALL override the configured startup maximum before the worker requests new jobs; when no persisted value exists, the configured startup maximum SHALL apply. Lowering the target SHALL let active encodes finish before admitting another. The operator control interface SHALL remain local; server communication SHALL be limited to reading and writing that worker's persisted job maximum and SHALL NOT allow the server to issue arbitrary control commands to a worker.

#### Scenario: Read runtime admission status
- **WHEN** an operator runs the local status command
- **THEN** the worker reports whether admission is enabled, active and reserved jobs, the job maximum, active and target encodes, and whether the schedule permits requests

#### Scenario: Disable new job requests
- **WHEN** an operator sets the maximum to zero
- **THEN** the worker persists zero for its configured worker name, stops new requests, and leaves accepted and restored jobs running, including their later FFmpeg phase at the previous positive encode target

#### Scenario: A request is already in flight
- **WHEN** an operator sets the maximum to zero while a reserved request is waiting for the server
- **THEN** the worker reports that reservation and does not start further requests; the server may still assign the reserved job

#### Scenario: Change the maximum at runtime
- **WHEN** an operator sets a positive maximum different from the current value
- **THEN** the worker persists the value for its configured worker name and applies it to future job requests and FFmpeg encode concurrency without restarting

#### Scenario: Lower the encode maximum without interruption
- **WHEN** an operator lowers the maximum below the number of FFmpeg jobs already running
- **THEN** those encodes continue and no new encode starts until active encodes fall below the new target

#### Scenario: Restore a persisted maximum after restart
- **WHEN** a worker restarts with a persisted maximum for its configured worker name
- **THEN** it applies that value before requesting new jobs, even when its startup configuration specifies another maximum

#### Scenario: Use startup maximum when no override exists
- **WHEN** a worker starts and the server has no persisted maximum for its configured worker name
- **THEN** it uses its configured startup maximum

#### Scenario: Server unavailable during persistence
- **WHEN** an operator changes the maximum while the server cannot persist the request
- **THEN** the local control reports an error and does not report the change as successful

#### Scenario: Reject invalid maximum
- **WHEN** an operator supplies a negative or non-integer maximum
- **THEN** the command reports an error and leaves the current and persisted values unchanged
