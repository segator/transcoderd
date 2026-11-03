# Spec Delta

## ADDED Requirements

### Requirement: Graceful manual Windows upgrade wait
Manual Setup SHALL use its bundled helper to close CPU and GPU admission and wait for all accepted work, including queued encodes and uploads, before stopping the service or replacing files. It SHALL display remaining work, preserve settings and never force-stop jobs or arm PC shutdown. Waiting SHALL be bounded to 24 hours; errors or timeout SHALL leave files and active work intact.

#### Scenario: CPU is busy and GPU can accept work
- **WHEN** manual Setup starts with accepted CPU work and an available GPU profile
- **THEN** both profiles stop admission, accepted work finishes, and Setup continues automatically after both confirm idle

#### Scenario: Queued or RAM-paused accepted work
- **WHEN** Setup waits with queued or suspended accepted encodes
- **THEN** those encodes resume and finish including verification and upload while new requests remain blocked

#### Scenario: Upgrade from an older service
- **WHEN** the installed helper has the previous immediate-refusal behavior
- **THEN** the new installer uses its own helper and existing worker controls to wait for safe installation

#### Scenario: Unknown activity or competing update
- **WHEN** a configured profile cannot report its activity or a mandatory automatic update is pending
- **THEN** manual Setup reports the error without stopping the service or replacing files

#### Scenario: Wait timeout or cancellation
- **WHEN** the helper wait is canceled or exceeds 24 hours
- **THEN** Setup does not stop active work or replace files; any existing admission hold permits accepted work to finish and persisted settings remain unchanged
