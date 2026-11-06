# Spec Delta

## ADDED Requirements

### Requirement: Reported worker software version
Workers SHALL include the running binary version in runtime telemetry. Authenticated fleet cards, overview summaries and selected resource details SHALL display that reported version. Offline workers SHALL retain it as last-reported information. Missing versions SHALL show Version unavailable, without inference from protocol capabilities, server version or release tags. Heartbeats from older workers SHALL remain accepted.

#### Scenario: Worker release changes
- **WHEN** a worker reports a new running version in its heartbeat
- **THEN** the dashboard shows that version after its normal refresh without a manual page reload

#### Scenario: Offline worker
- **WHEN** a recently seen worker with a recorded version goes offline
- **THEN** its basic card shows the last-reported version without enabling live controls

#### Scenario: Older worker without version
- **WHEN** runtime telemetry omits the version or is absent
- **THEN** the dashboard shows Version unavailable and continues showing the supported worker information
