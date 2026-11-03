# Spec Delta

## ADDED Requirements

### Requirement: Optional worker runtime telemetry
Worker presence SHALL retain optional runtime telemetry from Ping events, including encoder, CPU/GPU kind, admission and schedule state, active jobs, active encodes, and encode target. Presence freshness SHALL use server receipt time. Pings without telemetry SHALL remain accepted and indicate that runtime measurements are unavailable.

#### Scenario: Modern worker heartbeat
- **WHEN** a worker sends Ping with runtime status
- **THEN** the server updates presence and exposes that status to authenticated dashboard readers

#### Scenario: Legacy worker heartbeat
- **WHEN** a worker sends Ping without runtime status
- **THEN** presence still updates and the dashboard reports runtime measurements as unavailable

### Requirement: UTC server monitoring timestamps
New server-received job notifications, worker presence, and progress updates SHALL record UTC timestamps independent of the server or database session timezone. Dashboard time buckets and freshness SHALL use UTC instants. Existing naive historical timestamps SHALL remain unchanged because their original timezone cannot be inferred reliably.

#### Scenario: Server uses a non-UTC timezone
- **WHEN** a server configured with a non-UTC local timezone receives a job notification or presence update
- **THEN** a dashboard refresh reports the event at its actual receipt instant rather than shifting it by the local UTC offset
