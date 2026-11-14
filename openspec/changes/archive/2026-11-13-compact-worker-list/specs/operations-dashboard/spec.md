# Operations Dashboard Delta

## ADDED Requirements

### Requirement: Compact searchable Workers list
The Workers view SHALL show one expandable row per worker or matched Windows machine. Rows SHALL start collapsed and show identity, status and available live CPU/GPU active and configured encode counts. Search SHALL match worker and profile names case-insensitively. State filters SHALL offer All, Active, Paused and Offline, with All selected by default. Search and state filters SHALL apply to the selected reporting period before pagination.

#### Scenario: Find a worker by name
- **WHEN** the operator searches for part of a worker or profile name
- **THEN** matching rows and their pagination reflect the search even if the rows were previously on a later page

#### Scenario: Filter by observed state
- **WHEN** the operator selects Active, Paused or Offline
- **THEN** Active includes machines with accepted work on an online profile, Paused includes machines with an online queue or RAM hold, and Offline includes machines with no online profile

#### Scenario: One Windows profile is disconnected
- **WHEN** only one profile in a matched Windows machine is online
- **THEN** the row identifies the partial outage and displays live capacity only for online profiles with telemetry

#### Scenario: Expand a row on mobile
- **WHEN** an operator expands an online worker at a mobile width
- **THEN** its tasks, compact resource graphs and controls are available in the row without page-level horizontal overflow
- **AND** a normal refresh preserves the expansion and unsaved capacity edits

#### Scenario: Follow a direct resource link
- **WHEN** the operator opens a worker resource link and the worker is in the selected reporting period
- **THEN** the view displays the requested resource detail and expands the associated worker row, independently of list search and state filters

## MODIFIED Requirements

### Requirement: Compact worker resource history
Expanded online worker rows SHALL show small graphs for available CPU, RAM, GPU and encoder usage according to their supported profile, using bounded samples received in the current tab. Missing or stale readings SHALL remain explicit and gaps SHALL not imply measured usage. Graphs SHALL be available within the expanded row.

#### Scenario: Profile card history
- **WHEN** distinct hardware samples arrive for a CPU or GPU Windows profile and the operator expands its worker row
- **THEN** the profile shows RAM history and only its relevant CPU or GPU/encoder history
