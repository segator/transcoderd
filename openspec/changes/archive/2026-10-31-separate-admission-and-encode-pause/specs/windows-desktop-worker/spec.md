# Spec Delta

## MODIFIED Requirements

### Requirement: Desktop encoding pause mode
Desktop settings SHALL persist a pause mode with `ram` displayed as "RAM pause"; legacy empty or finish desktop values SHALL migrate to ram. A keyboard-accessible tooltip SHALL explain that RAM pause retains RAM/VRAM, resumes without restarting encoding, and cannot survive process exit, shutdown or reboot. Mode-only edits SHALL apply without restarting encodes.

#### Scenario: Choose RAM pause
- **WHEN** the user presses Pause encodes
- **THEN** both CPU and GPU profiles suspend active encoding processes and the dashboard identifies suspended jobs as paused in RAM

#### Scenario: Restore saved settings
- **WHEN** settings are loaded after a service restart
- **THEN** the selected mode is retained, with missing or legacy finish desktop values interpreted as ram

#### Scenario: Explain resource retention
- **WHEN** the user hovers over or focuses the pause-mode help control
- **THEN** the tooltip explains retained RAM and VRAM and the inability to preserve running encoding across process exit or reboot

#### Scenario: Change mode during pause
- **WHEN** the user changes only the pause mode while paused
- **THEN** the strategy is saved without changing either general pause preference

### Requirement: Graceful desktop reconfiguration
Configuration changes requiring worker restart SHALL first stop admission locally without changing the persisted server job limit, then wait for accepted jobs and in-flight requests to finish before restarting. User-requested admission pause SHALL use persistent admission limits; RAM pause SHALL preserve its independent local preference.

#### Scenario: Change schedule while an encode runs
- **WHEN** the user saves a different schedule while jobs are active
- **THEN** new requests stop, active jobs finish, and the worker restarts with the new schedule

#### Scenario: Resume a paused worker
- **WHEN** the user sets a positive concurrency after pausing
- **THEN** the service persists and applies that limit using the existing worker control

## ADDED Requirements

### Requirement: Separate queue and encoding pause controls
The dashboard SHALL show Stop after current / Resume queue and Pause encodes / Resume encodes as separate controls, with equivalent tray actions. The former SHALL allow running encodes to finish; the latter SHALL immediately suspend them in RAM. Controls SHALL preserve each other's state and individual holds, remain locked during maintenance, and persist both desktop preferences independently.

#### Scenario: Both controls active
- **WHEN** the user stops after current and then pauses encodes
- **THEN** resuming encodes lets active work continue without reopening the queue; resuming the queue first leaves encoding suspended

#### Scenario: Restart with saved controls
- **WHEN** the service restarts with independently saved queue and RAM preferences
- **THEN** both are applied before restored encodes or admission can start, without claiming that live encoder progress survives restart

#### Scenario: Update draining
- **WHEN** maintenance drains a worker with both controls active
- **THEN** accepted work resumes and finishes, new assignments remain blocked and both saved preferences are preserved for the new installation
