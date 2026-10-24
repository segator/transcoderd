# Spec Delta

## REMOVED Requirements

### Requirement: Local admission policy

## ADDED Requirements

### Requirement: Local job admission limit
A worker SHALL request a new job only when its active-job count plus in-flight job reservations is below its maximum, the maximum is greater than zero, and any configured time window permits work. The default maximum SHALL be two active jobs. Changing the maximum at runtime SHALL affect future requests only; active and restored jobs SHALL continue, and a reduced limit SHALL prevent new requests until capacity is available.

#### Scenario: Default active-job limit
- **WHEN** no maximum is supplied at startup
- **THEN** the worker uses a maximum of two active jobs

#### Scenario: Custom active-job limit
- **WHEN** the maximum is `4` and three jobs are active
- **THEN** the worker can request another job

#### Scenario: Zero active-job limit
- **WHEN** the active-job maximum is set to zero at startup or runtime
- **THEN** the worker requests no new jobs and continues active or restored jobs

#### Scenario: Lower maximum while jobs are active
- **WHEN** an operator lowers the maximum below the current active-job count
- **THEN** active jobs continue and the worker requests no new jobs until capacity is available

#### Scenario: Raise maximum while worker is ready
- **WHEN** an operator raises the maximum above the active-job count
- **THEN** the worker may request jobs until the active-job count reaches the new maximum

#### Scenario: Limit within configured window
- **WHEN** local time is inside a configured interval but the active-job limit is reached
- **THEN** the worker does not request another job

#### Scenario: Outside configured window
- **WHEN** the local time is outside the configured interval
- **THEN** the worker does not request a job regardless of its active-job count

### Requirement: Runtime worker admission controls
A running worker SHALL provide a local control command to report admission state and active-job count, and to set the maximum active-job count to a non-negative integer. These controls SHALL be available without restarting or reconfiguring the worker container and SHALL NOT expose a remotely reachable management endpoint.

#### Scenario: Read runtime status
- **WHEN** an operator requests worker control status
- **THEN** the worker reports whether admission is enabled, the active-job count, and the current maximum

#### Scenario: Set the maximum to zero
- **WHEN** an operator sets the maximum to zero
- **THEN** the worker stops requesting jobs and leaves active jobs running

#### Scenario: A request is already in flight
- **WHEN** an operator sets the maximum to zero while a reserved request is waiting for the server
- **THEN** the worker reports the reservation and does not start further requests; the server may still assign the reserved job

#### Scenario: Set a positive maximum
- **WHEN** an operator sets the maximum to a positive integer
- **THEN** the worker updates the limit immediately for future job requests

#### Scenario: Reject an invalid maximum
- **WHEN** an operator sets a negative or non-integer maximum
- **THEN** the command reports an error and leaves the current maximum unchanged
