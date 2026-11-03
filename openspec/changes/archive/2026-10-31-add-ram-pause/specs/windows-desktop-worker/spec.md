# Spec Delta

## MODIFIED Requirements

### Requirement: Aggregated mixed-profile monitoring
The dashboard SHALL aggregate active jobs, progress, encode counts, and admission state across enabled CPU and GPU profiles while identifying each job's profile.

#### Scenario: Display simultaneous mixed-profile work
- **WHEN** CPU and GPU profiles encode jobs at the same time
- **THEN** the dashboard shows both jobs and their respective profiles and includes both in aggregate counts

#### Scenario: Pause and resume mixed profiles
- **WHEN** the user pauses or resumes the worker
- **THEN** new assignments and queued FFmpeg starts stop or resume for both profiles, while active encodes follow the configured pause mode and downstream work finishes

#### Scenario: Configured limits govern startup
- **WHEN** the desktop specifies one CPU and four GPU encodes and the server retains different old limits
- **THEN** both workers use the desktop limits before starting encodes or accepting assignments, with at most one additional pipeline job per enabled profile

## ADDED Requirements

### Requirement: Desktop encoding pause mode
Desktop settings SHALL persist a pause mode with `finish` as the default for existing settings and `ram` displayed as "RAM pause". A keyboard-accessible tooltip SHALL explain that RAM pause retains RAM/VRAM, resumes without restarting encoding, and cannot survive process exit, shutdown or reboot. Mode-only edits SHALL apply without restarting encodes.

#### Scenario: Choose RAM pause
- **WHEN** the user saves RAM pause and presses Pause
- **THEN** both CPU and GPU profiles suspend active encoding processes and the dashboard identifies suspended jobs as paused in RAM

#### Scenario: Restore saved settings
- **WHEN** settings are loaded after a service restart
- **THEN** the selected mode is retained, with missing legacy values interpreted as finish

#### Scenario: Explain resource retention
- **WHEN** the user hovers over or focuses the pause-mode help control
- **THEN** the tooltip explains retained RAM and VRAM and the inability to preserve running encoding across process exit or reboot

#### Scenario: Change mode during pause
- **WHEN** the user changes only the pause mode while paused
- **THEN** the mode is applied to both profiles without draining, restarting or admitting new jobs
