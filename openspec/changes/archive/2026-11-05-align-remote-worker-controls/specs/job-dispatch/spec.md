## ADDED Requirements

### Requirement: Worker-owned remote control application
Workers SHALL advertise remote-control support, receive authenticated pending commands through settings polling, apply them using their own process/admission controls and acknowledge the command ID with success or failure. Applied commands SHALL not be reapplied while acknowledgement retries. Queue controls SHALL persist limits before application; positive settings SHALL remain available for subsequent queue resume. Maintenance holds SHALL remain authoritative.

#### Scenario: Lost acknowledgement
- **WHEN** application succeeds but its acknowledgement request fails
- **THEN** the worker retries acknowledgement without repeating the control or resetting later local pause state

#### Scenario: Older worker
- **WHEN** a worker does not advertise remote-control support
- **THEN** new remote commands are rejected while existing settings reads and capacity changes remain compatible

#### Scenario: Queue stop preserves independent holds
- **WHEN** queue stop or resume is applied while RAM or individual holds exist
- **THEN** those holds survive and normal polling cannot reopen maintenance admission
