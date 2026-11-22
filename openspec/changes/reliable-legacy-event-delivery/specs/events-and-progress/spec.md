## MODIFIED Requirements

### Requirement: Sequential notification history

A job notification SHALL carry a phase type, status, worker name, message, and integer `eventID`. The server SHALL append a new event only when its `eventID` immediately follows the last stored event for that job. An identical replay SHALL be acknowledged without duplicating its effects. A conflicting event SHALL receive an explicit definitive rejection.

#### Scenario: Next notification
- **WHEN** a notification uses the next expected event number
- **THEN** the server stores it and updates the job's current status projection

#### Scenario: Exact replay after a lost response
- **WHEN** the worker retries an event already committed with the same identity and payload
- **THEN** the server acknowledges it without appending or repeating finalization

#### Scenario: Duplicate or skipped number
- **WHEN** a notification conflicts with a stored event or skips ahead
- **THEN** the server rejects it definitively without appending it to the history

## ADDED Requirements

### Requirement: Resilient notification delivery

The worker SHALL persist pending job notifications and retry transient network errors, request timeouts and server failures indefinitely without discarding accepted work. Each attempt SHALL have a bounded duration. Notification delivery for one job SHALL NOT block unrelated jobs. An ambiguous response SHALL NOT be classified as definitive rejection.

#### Scenario: Server temporarily unavailable
- **WHEN** the server is unreachable or returns a transient failure
- **THEN** the worker retains the job and retries its unchanged pending notification until acknowledged or shutdown

#### Scenario: Worker restarts with an uncertain notification
- **WHEN** a worker restarts after sending a notification whose acknowledgement was not received
- **THEN** it recovers and retries the same notification without discarding the job

#### Scenario: Definitive rejection
- **WHEN** the server explicitly rejects a notification because the assignment expired
- **THEN** the worker stops that job, records the reason, discards its workspace, and continues processing other jobs
