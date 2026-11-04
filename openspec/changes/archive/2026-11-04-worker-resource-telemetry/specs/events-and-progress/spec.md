## ADDED Requirements

### Requirement: Optional worker hardware telemetry
Workers SHALL publish optional timestamped CPU/RAM and visible NVIDIA device readings through existing runtime heartbeats. Collection SHALL be read-only, default to five-second sampling and bound GPU queries to two seconds/two MiB without blocking heartbeat or job execution. CPU percentage SHALL require counter deltas. Unknown/unsupported sensors SHALL be unavailable rather than zero, with host or processor-group CPU scope explicit.

#### Scenario: Supported worker
- **WHEN** a worker can read system counters and NVIDIA telemetry
- **THEN** its heartbeat and dashboard API include measured CPU usage, memory and available per-device GPU/encoder/decoder utilization, VRAM, temperature and power

#### Scenario: CPU warm-up or GPU failure
- **WHEN** a first CPU reading has no prior sample or a GPU query is absent, unsupported, fails or times out
- **THEN** those measurements are unavailable while other telemetry and job coordination continue

#### Scenario: Older worker
- **WHEN** a worker omits resource telemetry
- **THEN** presence, runtime controls and job handling remain compatible and hardware readings are not fabricated

#### Scenario: Worker stops
- **WHEN** worker execution shuts down
- **THEN** its sampler and any active GPU query are canceled
