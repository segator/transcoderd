# Spec Delta

## MODIFIED Requirements

### Requirement: Local admission policy
A paused worker SHALL not request jobs. A worker SHALL stop requesting jobs when its active-job count reaches `worker.maxActiveJobs`, whose registered default SHALL be `2`. If a time window is configured, the worker SHALL additionally request jobs only while local time elapsed since midnight lies between `startAfter` and `stopAfter`.

#### Scenario: Worker paused
- **WHEN** the worker is paused
- **THEN** it does not request another job

#### Scenario: Ordinary active-job limit
- **WHEN** no time window is configured and two jobs are active at the default limit
- **THEN** the worker does not request another job

#### Scenario: Custom active-job limit
- **WHEN** `worker.maxActiveJobs` is `4` and three jobs are active
- **THEN** the worker can request another job

#### Scenario: Limit within configured window
- **WHEN** local time is inside a configured interval but the active-job limit is reached
- **THEN** the worker does not request another job

#### Scenario: Outside configured window
- **WHEN** local time is outside the configured interval
- **THEN** the worker does not request a job

## ADDED Requirements

### Requirement: Bounded concurrent encoding
The worker SHALL process up to `worker.ffmpegConfig.parallelJobs` encoding jobs simultaneously, with a registered default of `2`, while preserving a separate workspace, progress, and outcome for each job.

#### Scenario: Two encode jobs queued
- **WHEN** two accepted jobs reach the encode phase and the limit is `2`
- **THEN** both encoding jobs can run concurrently and each retains its own output and progress

#### Scenario: Encode limit reached
- **WHEN** the configured number of encoding jobs is already running
- **THEN** additional encode jobs wait until a slot is available
