# Spec Delta

## MODIFIED Requirements

### Requirement: Runtime worker admission controls
Local controls SHALL persist operator limits before reporting success. Positive maxima SHALL set FFmpeg concurrency and resume queued encodes. Zero SHALL retain the positive target but stop requests and new encodes; running encodes and downstream work SHALL finish. Startup SHALL use server overrides for standalone workers and service-owned Windows limits. Maintenance draining SHALL finish accepted work without changing saved preferences. Controls SHALL remain local.

#### Scenario: Read runtime admission status
- **WHEN** an operator runs the local status command
- **THEN** the worker reports whether admission is enabled, active and reserved jobs, the job maximum, active and target encodes, and whether the schedule permits requests

#### Scenario: Disable new job requests
- **WHEN** an operator sets the maximum to zero
- **THEN** the worker stops new requests and lets active encodes, verification and uploads finish, and holds waiting FFmpeg jobs until a positive maximum resumes them

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

#### Scenario: Paused encode slot becomes free
- **WHEN** an active encode finishes while paused and another accepted job is ready to encode
- **THEN** that queued job remains waiting until the user resumes

#### Scenario: Separate maintenance drain
- **WHEN** maintenance drains a paused worker for configuration or installation
- **THEN** all accepted work may finish while new assignments remain blocked and the saved pause preference remains intact

#### Scenario: Restore a persisted maximum after restart
- **WHEN** a standalone worker restarts with a persisted maximum for its configured worker name
- **THEN** it applies that value before requesting new jobs, even when its startup configuration specifies another maximum

#### Scenario: Use startup maximum when no override exists
- **WHEN** a standalone worker starts and the server has no persisted maximum for its configured worker name
- **THEN** it uses its configured startup maximum

#### Scenario: Server unavailable during persistence
- **WHEN** an operator changes the maximum while the server cannot persist the request
- **THEN** the local control reports an error and does not report the change as successful
