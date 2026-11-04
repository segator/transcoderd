# Spec Delta

## ADDED Requirements

### Requirement: Independent CPU and GPU worker slots
A persisted CPU/GPU pair SHALL override the single-limit policy for that worker. The worker SHALL bound each encoder class independently and admit at most their positive sum plus two pipeline jobs. One disabled class SHALL not accept new unassigned encodes. Disabling both SHALL stop new admission while accepted work drains. Lower positive limits SHALL not interrupt running work.

#### Scenario: Both encoder classes are enabled
- **WHEN** one worker has CPU limit 2 and GPU limit 3 and ready encoding jobs
- **THEN** up to two CPU and three GPU encodes run concurrently within that process

#### Scenario: Disable CPU while GPU remains enabled
- **WHEN** an operator changes CPU to zero while leaving GPU positive
- **THEN** active CPU encodes finish, new unassigned encodes use GPU slots, and GPU admission continues

#### Scenario: Pause both classes
- **WHEN** both desired limits become zero
- **THEN** new admission stops and accepted jobs continue through retained per-class drain capacity

#### Scenario: Restore pinned work on a disabled class
- **WHEN** a saved CPU or GPU encode is restored while that class has desired zero
- **THEN** it resumes with its pinned profile using bounded drain capacity while no new job is assigned to that class

#### Scenario: Legacy startup or local control
- **WHEN** no pair is stored or a positive legacy single-limit control replaces the pair
- **THEN** the startup selected encoder and existing single-limit behavior apply
