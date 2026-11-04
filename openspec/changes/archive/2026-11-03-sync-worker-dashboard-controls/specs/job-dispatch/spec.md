# Spec Delta

## ADDED Requirements

### Requirement: Bidirectional operator capacity synchronization
Local web and CLI controls SHALL persist atomic CPU/GPU pairs before applying them. Local and fleet controls SHALL share per-name desired settings with last committed writes winning. Polling SHALL converge approximately every five seconds, including while paused. Failed writes SHALL leave runtime limits unchanged. Managed startup SHALL honor stored overrides and use local defaults only when absent.

#### Scenario: Local change reaches fleet
- **WHEN** an operator saves CPU 2 and GPU 3 from the worker UI
- **THEN** the server stores that pair, the worker applies it, and fleet telemetry reports the observed pair

#### Scenario: Managed restart after server change
- **WHEN** a managed profile starts after a newer desired limit was saved on the server
- **THEN** startup applies the stored value without overwriting it with an older service configuration

### Requirement: Capacity changes preserve independent holds
Changing limits SHALL preserve independent RAM and individual encoding holds. Queue stop at zero SHALL block assignment and queued FFmpeg starts while active/downstream work finishes. Maintenance and explicit finish holds SHALL survive settings polling. The accepted-job ceiling SHALL be the encoder target plus one pipeline job.

#### Scenario: RAM pause and limit change
- **WHEN** limits change while a worker is RAM-paused
- **THEN** the new limits apply without clearing the RAM hold or individual holds

#### Scenario: Maintenance or finish polling
- **WHEN** persisted positive settings are polled during maintenance draining or explicit finishing
- **THEN** no new admission opens and accepted work remains allowed to drain


### Requirement: Supervised Windows encoder profiles
Windows service profile workers SHALL retain their configured CPU or GPU class and a maximum of 64 slots. Fleet controls SHALL identify these profiles and reject enabling the other encoder class on the same profile. Standalone workers SHALL retain independent dual-class support.

#### Scenario: CPU Windows profile limits
- **WHEN** fleet controls save GPU slots on a supervised CPU profile
- **THEN** the request is rejected without changing persisted limits and the GPU input is disabled in that profile's card


## MODIFIED Requirements

### Requirement: Runtime worker admission controls
Local controls SHALL persist operator limits before reporting success. Positive maxima SHALL update concurrency and resume queued encodes. Zero SHALL stop requests and new encodes while preserving targets; active encodes SHALL use pauseMode and downstream work SHALL finish. Startup SHALL honor persisted overrides or keep local defaults when no override exists. Maintenance draining SHALL finish accepted work without changing preferences. Controls SHALL remain local.

#### Scenario: Read runtime admission status
- **WHEN** an operator runs the local status command
- **THEN** the worker reports whether admission is enabled, active and reserved jobs, the job maximum, active and target encodes, and whether the schedule permits requests

#### Scenario: Disable new job requests
- **WHEN** an operator sets the maximum to zero
- **THEN** the worker stops new requests and applies the configured pause mode to active encodes and lets verification and uploads finish, and holds waiting FFmpeg jobs until a positive maximum resumes them

#### Scenario: A request is already in flight
- **WHEN** an operator sets the maximum to zero while a reserved request is waiting for the server
- **THEN** the worker reports that reservation and does not start further requests; the server may still assign the reserved job

#### Scenario: Change the maximum at runtime
- **WHEN** an operator sets a positive maximum different from the current value
- **THEN** the worker applies it to future job requests and FFmpeg encode concurrency without restarting

#### Scenario: Lower the encode maximum without interruption
- **WHEN** an operator lowers the maximum below the number of FFmpeg jobs already running
- **THEN** those encodes continue and no new encode starts until active encodes fall below the new target

#### Scenario: Reject invalid maximum
- **WHEN** an operator supplies a negative or non-integer maximum
- **THEN** the command reports an error and leaves the current maximum unchanged

#### Scenario: Paused encode slot becomes free
- **WHEN** an active encode finishes while paused and another accepted job is ready to encode
- **THEN** that queued job remains waiting until the user resumes

#### Scenario: Separate maintenance drain
- **WHEN** maintenance drains a paused worker for configuration or installation
- **THEN** all accepted work may finish while new assignments remain blocked and the saved pause preference remains intact

#### Scenario: Restore a persisted maximum after restart
- **WHEN** a standalone worker restarts with a persisted maximum for its configured worker name
- **THEN** it applies that value before requesting new jobs, even when its startup configuration specifies another maximum

#### Scenario: Use startup maximum when no override exists
- **WHEN** a standalone worker starts and the server has no persisted maximum for its configured worker name
- **THEN** it uses its configured startup maximum

#### Scenario: Server unavailable during persistence
- **WHEN** an operator changes the maximum while the server cannot persist the request
- **THEN** the local control reports an error and does not report the change as successful

### Requirement: Bounded concurrent encoding
The worker SHALL bound FFmpeg jobs by target and preserve per-job workspace, progress, and outcome. Startup target SHALL use positive `worker.ffmpegConfig.parallelJobs`, else `worker.maxActiveJobs` with minimum one for restored work. Positive runtime `set-max-jobs` SHALL replace the target. Each non-FFmpeg phase SHALL have one consumer.

#### Scenario: Two encode jobs queued
- **WHEN** two accepted jobs reach the encode phase and the limit is `2`
- **THEN** both encoding jobs can run concurrently and each retains its own output and progress

#### Scenario: Encode limit reached
- **WHEN** the configured number of encoding jobs is already running
- **THEN** additional encode jobs wait until a slot is available

#### Scenario: Runtime encode target overrides startup value
- **WHEN** an operator sets a positive max-jobs value different from the startup encode target
- **THEN** the FFmpeg target changes to that value while each non-FFmpeg phase continues to process one job at a time

#### Scenario: Default encode concurrency follows admission
- **WHEN** `worker.maxActiveJobs` is `4` and the encode concurrency override is omitted
- **THEN** up to four accepted jobs can encode simultaneously

#### Scenario: Desktop startup has stale server limits
- **WHEN** a Windows service worker starts with four configured GPU slots and a different persisted server maximum
- **THEN** the persisted maximum is applied before new admission, while the local service default applies only if no override exists

#### Scenario: Paused Windows startup
- **WHEN** the Windows service restarts with its saved pause preference and no newer desired resume has been saved
- **THEN** restored queued encodes do not start until Resume or maintenance draining

### Requirement: Independent CPU and GPU worker slots
A persisted CPU/GPU pair SHALL override the single-limit policy for that worker. The worker SHALL bound each encoder class independently and admit at most their positive sum plus one pipeline job. One disabled class SHALL not accept new unassigned encodes. Disabling both SHALL stop new admission and queued encode starts while running encodes and downstream work finish. Lower positive limits SHALL not interrupt running work.

#### Scenario: Both encoder classes are enabled
- **WHEN** one worker has CPU limit 2 and GPU limit 3 and ready encoding jobs
- **THEN** up to two CPU and three GPU encodes run concurrently within that process

#### Scenario: Disable CPU while GPU remains enabled
- **WHEN** an operator changes CPU to zero while leaving GPU positive
- **THEN** active CPU encodes finish, new unassigned encodes use GPU slots, and GPU admission continues

#### Scenario: Pause both classes
- **WHEN** both desired limits become zero
- **THEN** new admission and queued encode starts stop, running encodes finish, and pending encodes wait for Resume or maintenance draining

#### Scenario: Restore pinned work on a disabled class
- **WHEN** a saved CPU or GPU encode is restored while that class has desired zero
- **THEN** it retains its pinned profile and uses bounded drain capacity when the global queue is open, while no new job is assigned to that class

#### Scenario: Legacy startup or local control
- **WHEN** no pair is stored or a positive legacy single-limit control replaces the pair
- **THEN** the startup selected encoder and existing single-limit behavior apply
