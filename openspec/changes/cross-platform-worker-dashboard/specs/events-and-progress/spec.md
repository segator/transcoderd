## MODIFIED Requirements

### Requirement: Local pipeline queue snapshot
The worker SHALL report accepted jobs waiting or running in each pipeline phase and whether their source has been downloaded. The local snapshot SHALL include jobs held outside the channel while waiting for an encode slot, and SHALL include the configured video encoder identity when available.

#### Scenario: Prefetched encode waits for capacity
- **WHEN** a downloaded job is queued for FFmpeg while all encode slots are occupied
- **THEN** local status reports the job as queued, downloaded and waiting for FFmpeg

#### Scenario: Identify local job encoder
- **WHEN** the worker reports a local job snapshot
- **THEN** the job's configured encoder is included so local dashboards can identify its CPU/GPU profile
