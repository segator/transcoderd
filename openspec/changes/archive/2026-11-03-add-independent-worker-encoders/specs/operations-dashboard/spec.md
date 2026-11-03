# Spec Delta

## MODIFIED Requirements

### Requirement: Live monitoring and individual capacity
The dashboard SHALL refresh periodically and show job phase progress and ETA when available, worker freshness, CPU/GPU encoders, active counts, and independent desired limits. Operators SHALL save CPU and GPU limits atomically per worker. Zero SHALL disable that class for new unassigned encodes; both zero SHALL pause admission. Accepted work SHALL continue.

#### Scenario: Change one worker
- **WHEN** an authenticated operator saves CPU and GPU limits for one worker
- **THEN** the server persists that worker's pair, leaves other workers unchanged, and distinguishes desired from observed capacity until synchronization

#### Scenario: Missing or stale telemetry
- **WHEN** a worker has old heartbeat data or supplies no runtime telemetry
- **THEN** the dashboard labels its state and unavailable measurements rather than inventing CPU/GPU utilization
