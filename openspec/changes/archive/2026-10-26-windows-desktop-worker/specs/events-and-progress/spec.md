## ADDED Requirements

### Requirement: Local current task progress
The worker SHALL expose a local snapshot of active task identifiers, phases, percentage and estimated remaining time through its existing authenticated local control channel.

#### Scenario: Query active task
- **WHEN** a local authorized client requests status while a task is progressing
- **THEN** the response includes its current progress without waiting for the next server event

#### Scenario: Task finishes
- **WHEN** a task completes or fails
- **THEN** it is removed from the current-task snapshot

### Requirement: Local pipeline queue snapshot
The worker SHALL report accepted jobs waiting or running in each pipeline phase and whether their source has been downloaded. The local snapshot SHALL include jobs held outside the channel while waiting for an encode slot.

#### Scenario: Prefetched encode waits for capacity
- **WHEN** a downloaded job is queued for FFmpeg while all encode slots are occupied
- **THEN** local status reports the job as queued, downloaded and waiting for FFmpeg
