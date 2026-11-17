## ADDED Requirements

### Requirement: Bounded incomplete downloads
The worker SHALL stop after three incomplete source transfers or three failed source integrity checks for one assignment and report an actionable failure. The server SHALL NOT automatically requeue a job with that failure. Users MAY explicitly retry the failed job after resolving the cause.

#### Scenario: Repeated truncated transfer
- **WHEN** three source transfers end before their declared size
- **THEN** the worker fails the job with a diagnostic identifying repeated incomplete downloads and the server does not automatically requeue it

#### Scenario: Repeated verification failure
- **WHEN** three downloaded sources cannot be verified against the server checksum
- **THEN** the worker fails the job with a diagnostic identifying repeated verification failures and the server does not automatically requeue it

#### Scenario: Transient interruption
- **WHEN** an incomplete transfer is followed by a complete, verified transfer before the limit
- **THEN** the worker continues processing the verified source

### Requirement: Download progress per attempt
Download progress SHALL describe bytes received in the current attempt rather than accumulating bytes across attempts.

#### Scenario: A new download attempt starts
- **WHEN** the worker starts a replacement download attempt
- **THEN** its reported transferred byte count resets to zero
