# Spec Delta

## MODIFIED Requirements

### Requirement: Drain all accepted Windows work
Before replacing installation files, the service SHALL stop new assignments locally for both profiles and wait until all accepted jobs and reserved requests finish. Downloads, subtitle work, queued encodes, verification and uploads SHALL complete normally. An unreachable profile SHALL not be treated as idle. Maintenance draining SHALL preserve persisted server job limits and the user's pause and schedule settings.

#### Scenario: Encode finishes while its upload is running
- **WHEN** encoding has completed but an accepted upload remains active
- **THEN** the installer is not started until that upload and all other accepted work finish

#### Scenario: Both profiles are busy
- **WHEN** CPU and GPU profiles have accepted work at update time
- **THEN** neither receives further assignments and installation waits for both profiles and reserved requests to drain

#### Scenario: Existing pause or schedule
- **WHEN** an update completes for a paused worker or outside its admission schedule
- **THEN** the service retains that pause or schedule and does not admit jobs contrary to it

#### Scenario: User paused before the update
- **WHEN** an authenticated mandatory update is detected while user pause holds prefetched encodes
- **THEN** maintenance permits accepted work to finish without accepting new assignments and the upgraded worker retains the user's saved pause
