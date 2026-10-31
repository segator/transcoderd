# Spec Delta

## MODIFIED Requirements

### Requirement: Selectable encoding pause mode
The worker SHALL accept `worker.pauseMode` as `finish` (default) or `ram`. A zero runtime maximum SHALL hold new assignments and queued encodes. In `ram` mode on Windows and Linux it SHALL also suspend owned active FFmpeg encoding processes, retaining RAM and GPU memory; a positive maximum SHALL resume encodes without an individual pause. Other modes SHALL be rejected.

#### Scenario: Suspend and resume CPU or GPU encoding
- **WHEN** a worker using RAM pause receives a zero maximum while CPU or GPU encodes run
- **THEN** it suspends those processes and reports suspended encodes without discarding progress; a positive maximum resumes the same processes except those paused individually

#### Scenario: Default finish behavior
- **WHEN** pause mode is omitted or `finish` and the maximum becomes zero
- **THEN** active encodes without an individual hold finish while queued encodes remain held

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
- **THEN** switching to RAM suspends active encodes and switching to finish resumes active encodes without an individual pause, without starting queued encodes

## ADDED Requirements

### Requirement: Individual active encode pause
Local controls SHALL pause or resume one owned active encoding by job ID on Windows/Linux without changing other encodes, job limits or admission. Individual pause SHALL retain RAM/VRAM and occupied encode slots, use the same process on resume and remain independent of the configured general pause mode. Unknown or completed encodes SHALL be rejected.

#### Scenario: Pause one CPU or GPU encode
- **WHEN** an operator pauses one active encoding while another is running
- **THEN** only the chosen encoding stops progressing and the other continues with unchanged limits

#### Scenario: General pause composes with individual pause
- **WHEN** an individually paused encode is followed by general RAM pause and general Resume
- **THEN** the individually paused encode remains stopped and encodes held only by the general pause resume

#### Scenario: Individual resume during general RAM pause
- **WHEN** an individual resume is requested while general RAM pause remains active
- **THEN** the individual hold is cleared but the process remains suspended until the general pause is resumed

#### Scenario: Target has finished
- **WHEN** a control targets an unknown job or an encode that completed before the request was applied
- **THEN** it returns an error and affects no other process

#### Scenario: Maintenance drains individual holds
- **WHEN** configuration or update maintenance drains a worker with individually paused encodes
- **THEN** all suspended encodes resume, admission remains blocked and further individual pause requests are rejected during the drain
