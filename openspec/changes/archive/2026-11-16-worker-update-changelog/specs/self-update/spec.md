## ADDED Requirements

### Requirement: Windows worker release notes
The Windows worker dashboard SHALL offer release notes beside the update check. For a pending update it SHALL show stable release notes newer than the installed version up to and including the pending target. When no update is pending it SHALL show notes for the installed release. Notes are informational and SHALL NOT affect update eligibility or installation.

#### Scenario: Pending update spans releases
- **WHEN** an authenticated pending update is newer than the installed version
- **THEN** the dashboard displays the available notes for releases in that version range and identifies its endpoints

#### Scenario: No pending update
- **WHEN** no update is pending
- **THEN** the dashboard displays the notes for the installed version

#### Scenario: Notes unavailable or incomplete
- **WHEN** release notes or version information cannot be retrieved completely
- **THEN** the dashboard reports the limitation and the mandatory update continues independently

#### Scenario: Update target changes while notes are open
- **WHEN** the installed or pending version changes
- **THEN** the dashboard refreshes the displayed range without showing an older request as the current result
