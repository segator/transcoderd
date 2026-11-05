## ADDED Requirements

### Requirement: Explicit acknowledged remote worker controls
Online capable workers SHALL offer Pause in RAM/Resume encodes separately from Stop after current/Resume queue. Commands SHALL require authenticated same-origin requests and worker acknowledgement, with pending/applied/failed feedback. Queue resume SHALL restore the last positive limits. RAM resume SHALL preserve queue and individual holds. Unsupported/offline/maintenance workers SHALL not accept these commands.

#### Scenario: Pause a running encode remotely
- **WHEN** an operator selects Pause in RAM
- **THEN** the worker suspends owned encodes through its local RAM control, retains limits and memory, acknowledges application, and the UI offers Resume encodes

#### Scenario: Stop and resume queue
- **WHEN** an operator selects Stop after current then Resume queue
- **THEN** active encodes finish while queued starts and admission stop, then the previous positive limits are restored without clearing independent RAM or individual holds

#### Scenario: Failed or superseded command
- **WHEN** a control fails or a newer command supersedes an unacknowledged command
- **THEN** the current command reports failure or pending accurately and an old acknowledgement cannot complete the newer command

### Requirement: Reporting-period worker visibility
The Workers view SHALL include only workers seen within its selected 24-hour, 7-day or 30-day reporting period. Fleet-wide membership SHALL retain its 30-day maximum. Filtered worker counts and pagination SHALL describe the visible set.

#### Scenario: Last 24 hours
- **WHEN** Last 24 hours is selected and a worker was last seen two days ago
- **THEN** it is absent from Workers while a worker seen within 24 hours remains visible

### Requirement: Compact worker resource history
Online worker cards SHALL show small graphs for available CPU, RAM, GPU and encoder usage according to their supported profile, using bounded samples received in the current tab. Missing/stale readings SHALL remain explicit and gaps SHALL not imply measured usage. Graphs SHALL not require opening worker details.

#### Scenario: Profile card history
- **WHEN** distinct hardware samples arrive for a CPU or GPU Windows profile
- **THEN** its card shows RAM history and only its relevant CPU or GPU/encoder history
