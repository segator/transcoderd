# Spec Delta

## MODIFIED Requirements

### Requirement: Selectable encoding pause mode
The worker SHALL accept `worker.pauseMode` as `finish` (default) or `ram`. A zero runtime maximum SHALL hold new assignments and queued encodes. In `ram` mode on Windows and Linux it SHALL also suspend owned active FFmpeg encoding processes, retaining RAM and GPU memory; a positive maximum SHALL release its legacy RAM hold without clearing other holds. Other modes SHALL be rejected.

#### Scenario: Suspend and resume CPU or GPU encoding
- **WHEN** a worker using RAM pause receives a zero maximum while CPU or GPU encodes run
- **THEN** it suspends those processes and reports suspended encodes without discarding progress; a positive maximum releases its legacy hold while explicit RAM and individual holds remain effective

#### Scenario: Default finish behavior
- **WHEN** pause mode is omitted or `finish` and the maximum becomes zero
- **THEN** active encodes without other holds finish while queued encodes remain held

#### Scenario: Start and exit races
- **WHEN** pause races an encoder start or completion
- **THEN** the worker holds new starts during RAM pause and never suspends an unrelated process

#### Scenario: Suspension failure
- **WHEN** the OS refuses to suspend or resume an owned encoding process
- **THEN** control reports an error instead of claiming a successful transition

#### Scenario: Maintenance drain
- **WHEN** maintenance drains a RAM-paused worker
- **THEN** suspended and queued accepted encodes may finish while admission stays blocked and the saved user pause remains intact

#### Scenario: Shutdown while paused
- **WHEN** the worker shuts down while an encoding process is suspended
- **THEN** normal cancellation terminates the owned process without requiring Resume

#### Scenario: Live mode change
- **WHEN** the mode changes while the worker is paused
- **THEN** switching to RAM suspends active encodes and switching to finish releases its legacy hold without clearing explicit RAM or individual holds, or starting queued encodes

## ADDED Requirements

### Requirement: Independent admission and RAM holds
Explicit admission controls SHALL stop requests and queued encode starts while allowing active encodes to finish, without applying the legacy zero-limit RAM policy. Explicit RAM controls SHALL suspend active encodes and hold requests and encode starts without changing the job limit. Releasing either control SHALL preserve the other and individual holds. Maintenance SHALL temporarily release RAM holds while keeping admission closed.

#### Scenario: Stop after current
- **WHEN** an operator stops the queue through the explicit admission control
- **THEN** running encodes continue and no new requests or queued encodes start, even if legacy pauseMode was ram

#### Scenario: Pause encodes immediately
- **WHEN** the operator activates RAM pause while work is running
- **THEN** owned active encodes suspend immediately and requests and queued encodes are held without changing the configured job limit

#### Scenario: Resume RAM while queue is stopped
- **WHEN** both controls are active and the operator resumes RAM-paused encodes
- **THEN** active encodes without individual holds resume, while requests and queued encodes remain held

#### Scenario: Resume queue while RAM is paused
- **WHEN** both controls are active and the operator resumes the queue
- **THEN** configured limits are restored but requests and encode starts remain held until RAM pause is released

#### Scenario: Startup RAM preference
- **WHEN** worker.ramPaused is true at startup
- **THEN** restored encodes and new assignments remain held until the explicit RAM control resumes; the default is false
